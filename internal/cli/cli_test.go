package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bttnns/joblog/internal/store"
	"github.com/spf13/cobra"
)

// run executes the root command with a fixed data dir and returns any error.
func run(t *testing.T, dir string, args ...string) error {
	t.Helper()
	full := append([]string{"--data-dir", dir}, args...)
	root := NewRootCmd()
	root.SetArgs(full)
	return root.Execute()
}

// runCapture runs a command with stdout redirected to a pipe and returns what it
// wrote, for asserting on --json shapes and emitted prompts. Not parallel-safe
// (it swaps os.Stdout), which is fine for these sequential tests.
func runCapture(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	runErr := run(t, dir, args...)
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), runErr
}

// TestCommandTree guards against a cobra Use string whose first word is not the
// command name (which silently misnames a subcommand).
func TestCommandTree(t *testing.T) {
	root := NewRootCmd()
	want := map[string][]string{
		"log":     {"add", "ls", "show", "update", "rm"},
		"role":    {"import", "ls", "changes", "show", "rm"},
		"company": {"add", "search", "ls", "set", "rm", "show"},
		"report":  {"states"},
		"config":  {"set"},
		"profile": {"show", "build", "edit", "prompt"},
		"resume":  {"ls", "set", "add", "show", "diff", "rm"},
	}
	for parentName, kids := range want {
		parent := findCmd(root.Commands(), parentName)
		if parent == nil {
			t.Errorf("missing top-level command %q", parentName)
			continue
		}
		for _, kid := range kids {
			if findCmd(parent.Commands(), kid) == nil {
				t.Errorf("command %q missing subcommand %q", parentName, kid)
			}
		}
	}
}

// TestSkillCommandsResolve is the skill<->CLI contract: every `jl ...` command
// invocation written in the skill's SKILL.md and prompt files must resolve to a
// real command in the tree, and no stale `joblog ...` invocations may remain.
// This catches command renames (the jl regroup) that would otherwise leave the
// agent's prompts pointing at dead commands, with nothing else to flag it. Only
// code regions (inline backticks and fenced blocks) are inspected, so prose that
// merely mentions the tool is not treated as a command.
func TestSkillCommandsResolve(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	skillDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "skill")
	files, _ := filepath.Glob(filepath.Join(skillDir, "*.md"))
	prompts, _ := filepath.Glob(filepath.Join(skillDir, "prompts", "*.md"))
	files = append(files, prompts...)
	if len(files) == 0 {
		t.Skip("no skill files found")
	}

	root := NewRootCmd()
	inlineRe := regexp.MustCompile("`[^`]*`")
	fenceRe := regexp.MustCompile("(?s)```.*?```")
	invokeRe := regexp.MustCompile(`\bjl ([a-z][a-z-]*)(?: ([a-z][a-z-]*))?`)
	staleRe := regexp.MustCompile(`\bjoblog [a-z]`)

	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(f)
		// Restrict to code regions so prose mentions of the tool are ignored.
		code := strings.Join(append(inlineRe.FindAllString(string(b), -1), fenceRe.FindAllString(string(b), -1)...), "\n")

		if m := staleRe.FindString(code); m != "" {
			t.Errorf("%s: stale binary name in a command (%q); use jl", name, m)
		}
		for _, mm := range invokeRe.FindAllStringSubmatch(code, -1) {
			verb, sub := mm[1], mm[2]
			parent := findCmd(root.Commands(), verb)
			if parent == nil {
				t.Errorf("%s: `jl %s` is not a command", name, verb)
				continue
			}
			// Enforce the subcommand only for pure groups (no runnable parent),
			// so flag/arg words after a runnable verb are not misread as subcommands.
			if sub != "" && parent.RunE == nil && parent.Run == nil && len(parent.Commands()) > 0 {
				if findCmd(parent.Commands(), sub) == nil {
					t.Errorf("%s: `jl %s %s` is not a subcommand", name, verb, sub)
				}
			}
		}
	}
}

func findCmd(cmds []*cobra.Command, name string) *cobra.Command {
	for _, c := range cmds {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func TestEndToEndTracking(t *testing.T) {
	// Pin "now" so dates are deterministic.
	old := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = old }()

	dir := filepath.Join(t.TempDir(), "private")

	if err := run(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := run(t, dir, "add", "--employer", "Acme", "--title", "SRE", "--method", "online-portal"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := run(t, dir, "add", "--type", "networking", "--employer", "Globex", "--method", "linkedin"); err != nil {
		t.Fatalf("add networking: %v", err)
	}

	s := &store.Store{Dir: dir}
	log, err := s.LoadLog()
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 2 {
		t.Fatalf("want 2 entries, got %d", len(log))
	}
	if log[0].Date != "2026-03-18" {
		t.Errorf("entry date = %s, want pinned 2026-03-18", log[0].Date)
	}

	// Update the first entry's status, then verify.
	id := log[0].ID
	if err := run(t, dir, "update", id, "--status", "screen"); err != nil {
		t.Fatalf("update: %v", err)
	}
	log, _ = s.LoadLog()
	if log[0].Status != "screen" {
		t.Errorf("status = %s, want screen", log[0].Status)
	}

	// Remove the networking entry.
	if err := run(t, dir, "rm", log[1].ID); err != nil {
		t.Fatalf("rm: %v", err)
	}
	log, _ = s.LoadLog()
	if len(log) != 1 {
		t.Fatalf("after rm want 1 entry, got %d", len(log))
	}

	// Invalid enum should error.
	if err := run(t, dir, "add", "--type", "bogus"); err == nil {
		t.Error("add with bad --type: want error")
	}
}

func TestRolesImportAndConfig(t *testing.T) {
	old := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = old }()

	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}

	payload := `[
	  {"global_id":"greenhouse:1","title":"Senior SRE","company":"Acme","location":"Austin, TX","url":"https://x/1","salary_summary":"$200k","is_remote":true},
	  {"global_id":"greenhouse:2","title":"Backend Engineer","company":"Acme","location":"Remote","url":"https://x/2"}
	]`
	pf := filepath.Join(t.TempDir(), "roles.json")
	if err := os.WriteFile(pf, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "roles", "import", pf, "--company", "Acme"); err != nil {
		t.Fatalf("roles import: %v", err)
	}
	s := &store.Store{Dir: dir}
	roles, err := s.LoadRoles()
	if err != nil || len(roles) != 2 {
		t.Fatalf("roles = %d, %v", len(roles), err)
	}

	// Config set/get for state.
	if err := run(t, dir, "config", "set", "state", "tx"); err != nil {
		t.Fatalf("config set state: %v", err)
	}
	cfg, _ := s.LoadConfig()
	if cfg.State != "tx" {
		t.Errorf("config state = %q, want tx", cfg.State)
	}
	if err := run(t, dir, "config", "set", "state", "zz"); err == nil {
		t.Error("config set bad state: want error")
	}
}

// TestRoleImportRequiresCompany verifies the import command rejects a missing
// --company (which scopes gone-marking and links roles to a company).
func TestRoleImportRequiresCompany(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	pf := filepath.Join(t.TempDir(), "r.json")
	if err := os.WriteFile(pf, []byte(`[{"global_id":"x:1","title":"Eng","company":"Acme"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "role", "import", pf); err == nil {
		t.Error("role import without --company: want error")
	}
	if err := run(t, dir, "role", "import", pf, "--company", "acme"); err != nil {
		t.Errorf("role import with --company: unexpected error %v", err)
	}
}

// TestReportCheckExitContract pins the report --check contract: a short week
// returns ErrShortOfMinimum (which main maps to a distinct exit code), a met week
// returns nil, and --json --check emits the compliance view on stdout.
func TestReportCheckExitContract(t *testing.T) {
	old := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = old }()

	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "config", "set", "state", "tx"); err != nil {
		t.Fatal(err)
	}

	// Zero entries: short of the TX floor of 3.
	if err := run(t, dir, "report", "--check"); !errors.Is(err, ErrShortOfMinimum) {
		t.Fatalf("report --check (0 entries): err = %v, want ErrShortOfMinimum", err)
	}

	// Three applications meet the floor.
	for _, emp := range []string{"Acme", "Globex", "Initech"} {
		if err := run(t, dir, "add", "--employer", emp); err != nil {
			t.Fatal(err)
		}
	}
	if err := run(t, dir, "report", "--check"); err != nil {
		t.Fatalf("report --check (3 entries): err = %v, want nil", err)
	}

	out, err := runCapture(t, dir, "--json", "report", "--check")
	if err != nil {
		t.Fatalf("--json report --check: %v", err)
	}
	var v map[string]any
	if e := json.Unmarshal([]byte(out), &v); e != nil {
		t.Fatalf("not JSON: %v (%q)", e, out)
	}
	for _, k := range []string{"state", "week", "count", "required", "compliant", "source_url"} {
		if _, ok := v[k]; !ok {
			t.Errorf("compliance JSON missing key %q", k)
		}
	}
	if v["compliant"] != true {
		t.Errorf("compliant = %v, want true", v["compliant"])
	}
}

// TestAddSetsCompanySlug verifies the canonical company slug is stamped at write
// time: derived from --employer, overridable with --company, and inherited from a
// role via --from-role.
func TestAddSetsCompanySlug(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{Dir: dir}

	if err := run(t, dir, "add", "--employer", "Fastly Inc."); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "add", "--employer", "Fastly", "--company", "fastly"); err != nil {
		t.Fatal(err)
	}
	log, _ := s.LoadLog()
	if log[0].Company != "fastly-inc" {
		t.Errorf("derived company = %q, want fastly-inc", log[0].Company)
	}
	if log[1].Company != "fastly" {
		t.Errorf("explicit company = %q, want fastly", log[1].Company)
	}

	// Import a role under the slug "fastly" (its payload employer differs), then
	// add --from-role: the entry inherits the role's canonical slug.
	payload := `[{"global_id":"gh:1","title":"SRE","company":"Fastly Inc.","url":"https://x/1"}]`
	pf := filepath.Join(t.TempDir(), "r.json")
	if err := os.WriteFile(pf, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "roles", "import", pf, "--company", "fastly"); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "add", "--from-role", "gh:1"); err != nil {
		t.Fatal(err)
	}
	log, _ = s.LoadLog()
	last := log[len(log)-1]
	if last.Company != "fastly" {
		t.Errorf("from-role company = %q, want inherited fastly", last.Company)
	}
	if last.Employer != "Fastly Inc." {
		t.Errorf("from-role employer = %q, want inherited display name", last.Employer)
	}
}

// TestCompanyCRUD covers the company command surface (add/list/rm/show),
// including the eager-scaffolded research folder and the legacy `target` alias.
func TestCompanyCRUD(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{Dir: dir}

	// The `target` alias still resolves to `company`.
	if err := run(t, dir, "target", "add", "--name", "acme-corp", "--ats", "ashby", "--slug", "acme-corp"); err != nil {
		t.Fatalf("company add (via target alias): %v", err)
	}
	cs, _ := s.LoadCompanies()
	if len(cs) != 1 || cs[0].Name != "acme-corp" || cs[0].ATS != "ashby" {
		t.Fatalf("companies = %+v", cs)
	}
	// New companies start active.
	if cs[0].Status != store.StatusActive {
		t.Errorf("status = %q, want active", cs[0].Status)
	}
	// Eager-scaffolded research folder.
	if _, err := os.Stat(s.Path("companies", "acme-corp", "company.md")); err != nil {
		t.Errorf("company.md not scaffolded: %v", err)
	}
	if err := run(t, dir, "company", "add", "--ats", "ashby"); err == nil {
		t.Error("company add without --name or URL: want error")
	}
	// show resolves the company and reports its status and data columns.
	out, err := runCapture(t, dir, "--json", "company", "show", "acme-corp")
	if err != nil {
		t.Fatalf("company show: %v", err)
	}
	var sv map[string]any
	if e := json.Unmarshal([]byte(out), &sv); e != nil {
		t.Fatalf("show not JSON: %v (%q)", e, out)
	}
	if _, ok := sv["applied"]; !ok {
		t.Errorf("company show JSON missing applied key: %v", sv)
	}

	if err := run(t, dir, "company", "rm", "acme-corp"); err != nil {
		t.Fatalf("company rm: %v", err)
	}
	if cs, _ = s.LoadCompanies(); len(cs) != 0 {
		t.Errorf("after rm: %d companies, want 0", len(cs))
	}
}

// TestResumeCollection covers the resume noun group end to end: set the base,
// add a tailored variant for a role, ls them, show the extracted text, diff
// base against the variant, and rm the variant. Markdown files are used so no
// PDF tooling is needed.
func TestResumeCollection(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{Dir: dir}

	// Import a role so a tailored variant can be linked to it.
	payload := `[{"global_id":"greenhouse:42","title":"Staff SRE","company":"Acme","url":"https://x/42"}]`
	pf := filepath.Join(t.TempDir(), "roles.json")
	if err := os.WriteFile(pf, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "role", "import", pf, "--company", "Acme"); err != nil {
		t.Fatalf("role import: %v", err)
	}

	// Set the base resume.
	base := filepath.Join(t.TempDir(), "base.md")
	if err := os.WriteFile(base, []byte("# Jane\nSRE with 8y of distributed systems\nLikes Go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "resume", "set", base); err != nil {
		t.Fatalf("resume set: %v", err)
	}
	cfg, _ := s.LoadConfig()
	if cfg.ResumePath == "" {
		t.Fatal("resume set did not record ResumePath")
	}
	txt, found, err := readResumeText(s.Path("resume", "resume.txt"))
	if err != nil || !found || !strings.Contains(txt, "SRE with 8y") {
		t.Fatalf("resume.txt not written correctly: found=%v err=%v txt=%q", found, err, txt)
	}

	// Add a tailored variant for the role.
	tailored := filepath.Join(t.TempDir(), "tailored.md")
	if err := os.WriteFile(tailored, []byte("# Jane\nStaff SRE focused on reliability\nLikes Go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "resume", "add", "--role", "greenhouse:42", tailored); err != nil {
		t.Fatalf("resume add: %v", err)
	}
	// The variant is stored under the company folder with a sanitized id.
	if _, err := os.Stat(s.Path("companies", "acme", "resume-greenhouse-42.md")); err != nil {
		t.Errorf("tailored source not stored: %v", err)
	}
	if _, err := os.Stat(s.Path("companies", "acme", "resume-greenhouse-42.txt")); err != nil {
		t.Errorf("tailored text not stored: %v", err)
	}

	// ls --json: base plus one tailored variant resolving to the role.
	out, err := runCapture(t, dir, "--json", "resume", "ls")
	if err != nil {
		t.Fatalf("resume ls: %v", err)
	}
	var variants []resumeVariant
	if e := json.Unmarshal([]byte(out), &variants); e != nil {
		t.Fatalf("resume ls not JSON: %v (%q)", e, out)
	}
	if len(variants) != 2 {
		t.Fatalf("resume ls = %d variants, want 2: %+v", len(variants), variants)
	}
	var sawBase, sawTailored bool
	for _, v := range variants {
		if v.Kind == "base" {
			sawBase = true
		}
		if v.Kind == "tailored" {
			sawTailored = true
			if v.Title != "Staff SRE" {
				t.Errorf("tailored title = %q, want Staff SRE", v.Title)
			}
			if v.Company != "acme" {
				t.Errorf("tailored company = %q, want acme", v.Company)
			}
		}
	}
	if !sawBase || !sawTailored {
		t.Errorf("resume ls missing rows: base=%v tailored=%v", sawBase, sawTailored)
	}

	// show the tailored variant's text.
	out, err = runCapture(t, dir, "resume", "show", "greenhouse:42")
	if err != nil {
		t.Fatalf("resume show: %v", err)
	}
	if !strings.Contains(out, "Staff SRE focused on reliability") {
		t.Errorf("resume show = %q, want tailored text", out)
	}

	// diff base against the variant: the changed line shows up.
	out, err = runCapture(t, dir, "resume", "diff", "greenhouse:42")
	if err != nil {
		t.Fatalf("resume diff: %v", err)
	}
	if !strings.Contains(out, "-SRE with 8y of distributed systems") || !strings.Contains(out, "+Staff SRE focused on reliability") {
		t.Errorf("resume diff missing expected hunk:\n%s", out)
	}

	// rm base is refused.
	if err := run(t, dir, "resume", "rm", "base"); err == nil {
		t.Error("resume rm base: want error")
	}
	// rm the tailored variant.
	if err := run(t, dir, "resume", "rm", "greenhouse:42"); err != nil {
		t.Fatalf("resume rm: %v", err)
	}
	if _, err := os.Stat(s.Path("companies", "acme", "resume-greenhouse-42.md")); !os.IsNotExist(err) {
		t.Errorf("tailored source not removed: %v", err)
	}
}

// TestProfileSurface covers profile show/build/prompt: build emits the prompt
// carrying the resume text on stdout, prompt emits only the raw instruction
// block, and show prints profile.md.
func TestProfileSurface(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(t.TempDir(), "resume.md")
	if err := os.WriteFile(md, []byte("# Jane\nSRE with 8y of distributed systems\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// build with a file sets the base resume in the same step, then emits the
	// prompt with the resume text appended.
	out, err := runCapture(t, dir, "profile", "build", md)
	if err != nil {
		t.Fatalf("profile build: %v", err)
	}
	if !strings.Contains(out, "SRE with 8y") {
		t.Errorf("profile build prompt missing resume text (%d bytes)", len(out))
	}

	// prompt emits only the raw instruction block (no resume text appended).
	out, err = runCapture(t, dir, "profile", "prompt")
	if err != nil {
		t.Fatalf("profile prompt: %v", err)
	}
	if strings.Contains(out, "RESUME:") || strings.Contains(out, "SRE with 8y") {
		t.Errorf("profile prompt should be clean (no resume), got %q", out)
	}

	// show prints profile.md (scaffolded by init).
	out, err = runCapture(t, dir, "profile", "show")
	if err != nil {
		t.Fatalf("profile show: %v", err)
	}
	if len(strings.TrimSpace(out)) == 0 {
		t.Error("profile show printed nothing")
	}
}

// TestAddFromRoleLinksResume verifies log add --from-role auto-links a tailored
// resume when one exists for the role, and that --resume overrides it.
func TestAddFromRoleLinksResume(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{Dir: dir}

	payload := `[{"global_id":"greenhouse:7","title":"SRE","company":"Acme","url":"https://x/7"}]`
	pf := filepath.Join(t.TempDir(), "roles.json")
	if err := os.WriteFile(pf, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "role", "import", pf, "--company", "Acme"); err != nil {
		t.Fatal(err)
	}

	// No tailored resume yet: add --from-role leaves Resume empty.
	if err := run(t, dir, "add", "--from-role", "greenhouse:7"); err != nil {
		t.Fatalf("add from-role: %v", err)
	}
	log, _ := s.LoadLog()
	if log[len(log)-1].Resume != "" {
		t.Errorf("Resume = %q, want empty before any variant", log[len(log)-1].Resume)
	}

	// Store a tailored variant, then add --from-role auto-links it.
	tailored := filepath.Join(t.TempDir(), "tailored.md")
	if err := os.WriteFile(tailored, []byte("tailored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "resume", "add", "--role", "greenhouse:7", tailored); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "add", "--from-role", "greenhouse:7"); err != nil {
		t.Fatalf("add from-role with variant: %v", err)
	}
	log, _ = s.LoadLog()
	if got := log[len(log)-1].Resume; got != "greenhouse:7" {
		t.Errorf("Resume = %q, want auto-linked greenhouse:7", got)
	}

	// --resume overrides the auto-link.
	if err := run(t, dir, "add", "--from-role", "greenhouse:7", "--resume", "custom-path.md"); err != nil {
		t.Fatalf("add with --resume: %v", err)
	}
	log, _ = s.LoadLog()
	if got := log[len(log)-1].Resume; got != "custom-path.md" {
		t.Errorf("Resume = %q, want override custom-path.md", got)
	}
}
