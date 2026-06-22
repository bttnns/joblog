package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bttnns/joblog/internal/store"
)

// TestParseATSURL covers the multi-tenant host table: each confirmed host maps a
// board root or a deep posting URL to the right ats + slug, an unrecognized host
// stays custom (empty ats), and a non-URL is rejected.
func TestParseATSURL(t *testing.T) {
	cases := []struct {
		raw      string
		wantATS  string
		wantSlug string
	}{
		{"https://boards.greenhouse.io/acme", "greenhouse", "acme"},
		{"https://job-boards.greenhouse.io/globex/jobs/123", "greenhouse", "globex"},
		{"boards.eu.greenhouse.io/initech", "greenhouse", "initech"},
		{"https://jobs.ashbyhq.com/spacex", "ashby", "spacex"},
		{"https://jobs.ashbyhq.com/spacex/some-role-uuid", "ashby", "spacex"},
		{"https://jobs.lever.co/netflix", "lever", "netflix"},
		{"https://jobs.lever.co/netflix/abc-def", "lever", "netflix"},
		{"https://apply.workable.com/acme/", "workable", "acme"},
		{"https://jobs.workable.com/view/xyz", "workable", "view"},
		{"https://jobs.smartrecruiters.com/Acme", "smartrecruiters", "Acme"},
		{"https://acme.recruitee.com/o/some-role", "recruitee", "acme"},
		{"https://acme.recruitee.com", "recruitee", "acme"},
		{"https://acme.teamtailor.com/jobs/123", "teamtailor", "acme"},
		{"www.boards.greenhouse.io/acme", "greenhouse", "acme"},
		// Unrecognized hosts stay custom (single-tenant or unknown board).
		{"https://www.tesla.com/careers", "", ""},
		{"https://jobs.apple.com/en-us/search", "", ""},
		{"https://careers.somecorp.com/openings", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			m, err := parseATSURL(tc.raw)
			if err != nil {
				t.Fatalf("parseATSURL(%q): %v", tc.raw, err)
			}
			if m.ATS != tc.wantATS || m.Slug != tc.wantSlug {
				t.Errorf("parseATSURL(%q) = {ats:%q slug:%q}, want {ats:%q slug:%q}",
					tc.raw, m.ATS, m.Slug, tc.wantATS, tc.wantSlug)
			}
		})
	}

	// A bare non-URL value is rejected (so a flag value is not mistaken for a URL).
	if _, err := parseATSURL("acme"); err == nil {
		t.Error("parseATSURL(\"acme\"): want error for a non-URL")
	}
	// A recognized host with no slug path errors.
	if _, err := parseATSURL("https://boards.greenhouse.io/"); err == nil {
		t.Error("parseATSURL with no slug: want error")
	}
}

// TestCompanyAddURL covers adding companies by URL: the ATS and slug are parsed,
// the name is derived from the slug, and new companies start active.
func TestCompanyAddURL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{Dir: dir}

	if err := run(t, dir, "company", "add", "https://boards.greenhouse.io/acme-corp", "https://jobs.lever.co/globex"); err != nil {
		t.Fatalf("company add by URL: %v", err)
	}
	cs, _ := s.LoadCompanies()
	if len(cs) != 2 {
		t.Fatalf("companies = %d, want 2: %+v", len(cs), cs)
	}
	byName := map[string]store.Company{}
	for _, c := range cs {
		byName[c.Name] = c
	}
	acme, ok := byName["Acme Corp"]
	if !ok {
		t.Fatalf("missing Acme Corp (derived name); have %+v", cs)
	}
	if acme.ATS != "greenhouse" || acme.Slug != "acme-corp" || acme.Status != store.StatusActive {
		t.Errorf("acme = %+v, want greenhouse/acme-corp/active", acme)
	}
	globex, ok := byName["Globex"]
	if !ok {
		t.Fatalf("missing Globex; have %+v", cs)
	}
	if globex.ATS != "lever" || globex.Slug != "globex" {
		t.Errorf("globex = %+v, want lever/globex", globex)
	}

	// An unrecognized URL stays custom.
	if err := run(t, dir, "company", "add", "--name", "Weird", "https://careers.weird.com/jobs"); err != nil {
		t.Fatalf("company add custom URL: %v", err)
	}
	cs, _ = s.LoadCompanies()
	for _, c := range cs {
		if c.Name == "Weird" && c.ATS != "custom" {
			t.Errorf("weird ats = %q, want custom", c.ATS)
		}
	}
}

// TestCompanyStatusAndList covers the status field end to end: a new company is
// active, ls shows only active by default, set flips it to paused, --all then
// includes it, and the data columns (roles/applied) are present in JSON.
func TestCompanyStatusAndList(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{Dir: dir}

	for _, name := range []string{"alpha", "beta"} {
		if err := run(t, dir, "company", "add", "--name", name, "--ats", "ashby", "--slug", name); err != nil {
			t.Fatal(err)
		}
	}
	// An application against alpha drives the APPLIED column.
	if err := run(t, dir, "add", "--employer", "alpha"); err != nil {
		t.Fatal(err)
	}

	// Default ls: both active.
	out, err := runCapture(t, dir, "--json", "company", "ls")
	if err != nil {
		t.Fatalf("company ls: %v", err)
	}
	var rows []companyRow
	if e := json.Unmarshal([]byte(out), &rows); e != nil {
		t.Fatalf("company ls not JSON: %v (%q)", e, out)
	}
	if len(rows) != 2 {
		t.Fatalf("default ls = %d rows, want 2 (both active)", len(rows))
	}
	for _, r := range rows {
		if r.Name == "alpha" && r.Applied != 1 {
			t.Errorf("alpha applied = %d, want 1", r.Applied)
		}
	}

	// Pause beta.
	if err := run(t, dir, "company", "set", "beta", "paused"); err != nil {
		t.Fatalf("company set paused: %v", err)
	}
	cs, _ := s.LoadCompanies()
	for _, c := range cs {
		if c.Name == "beta" && c.Status != store.StatusPaused {
			t.Errorf("beta status = %q, want paused", c.Status)
		}
	}

	// Default ls now hides beta.
	out, _ = runCapture(t, dir, "--json", "company", "ls")
	rows = nil
	_ = json.Unmarshal([]byte(out), &rows)
	if len(rows) != 1 || rows[0].Name != "alpha" {
		t.Errorf("default ls after pause = %+v, want only alpha", rows)
	}

	// --all includes the paused one.
	out, _ = runCapture(t, dir, "--json", "company", "ls", "--all")
	rows = nil
	_ = json.Unmarshal([]byte(out), &rows)
	if len(rows) != 2 {
		t.Errorf("--all ls = %d rows, want 2", len(rows))
	}

	// An invalid status value is rejected.
	if err := run(t, dir, "company", "set", "alpha", "bogus"); err == nil {
		t.Error("company set with invalid status: want error")
	}
}

// TestCompanyStatusBackfill verifies a company written without a status (the
// pre-PR4 shape) reads back as active.
func TestCompanyStatusBackfill(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{Dir: dir}
	// Write a company with an empty Status, simulating legacy data.
	if err := s.SaveCompanies([]store.Company{{Name: "legacy", ATS: "ashby", Slug: "legacy"}}); err != nil {
		t.Fatal(err)
	}
	cs, err := s.LoadCompanies()
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].Status != store.StatusActive {
		t.Errorf("backfilled status = %q, want active", cs[0].Status)
	}
}

// TestRoleRm removes a role from the index by id.
func TestRoleRm(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{Dir: dir}

	payload := `[{"global_id":"gh:1","title":"SRE","company":"Acme","url":"https://x/1"},{"global_id":"gh:2","title":"Eng","company":"Acme","url":"https://x/2"}]`
	pf := filepath.Join(t.TempDir(), "r.json")
	if err := os.WriteFile(pf, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "role", "import", pf, "--company", "Acme"); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "role", "rm", "gh:1"); err != nil {
		t.Fatalf("role rm: %v", err)
	}
	rs, _ := s.LoadRoles()
	if len(rs) != 1 || rs[0].GlobalID != "gh:2" {
		t.Errorf("after rm roles = %+v, want only gh:2", rs)
	}
	if err := run(t, dir, "role", "rm", "nope:9"); err == nil {
		t.Error("role rm of unknown id: want error")
	}
}

// TestLogShow shows one entry's full detail.
func TestLogShow(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "add", "--employer", "Acme", "--title", "SRE", "--notes", "warm intro"); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{Dir: dir}
	log, _ := s.LoadLog()
	id := log[0].ID

	out, err := runCapture(t, dir, "log", "show", id)
	if err != nil {
		t.Fatalf("log show: %v", err)
	}
	if !strings.Contains(out, "Acme") || !strings.Contains(out, "warm intro") {
		t.Errorf("log show missing detail: %q", out)
	}
	if err := run(t, dir, "log", "show", "zzzz"); err == nil {
		t.Error("log show of unknown id: want error")
	}
}

// TestFetchSkipsPaused verifies fetch with no args skips paused companies, while
// a named paused company is fetched anyway.
func TestFetchSkipsPaused(t *testing.T) {
	companies := []store.Company{
		{Name: "active-co", ATS: "ashby", Slug: "active-co", Status: store.StatusActive},
		{Name: "paused-co", ATS: "ashby", Slug: "paused-co", Status: store.StatusPaused},
	}
	// No args: only the active company is a target.
	targets, err := resolveFetchTargets(companies, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Name != "active-co" {
		t.Errorf("default targets = %+v, want only active-co", targets)
	}
	// --paused includes both.
	targets, _ = resolveFetchTargets(companies, nil, true)
	if len(targets) != 2 {
		t.Errorf("--paused targets = %d, want 2", len(targets))
	}
	// A named paused company is fetched regardless of status.
	targets, err = resolveFetchTargets(companies, []string{"paused-co"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Name != "paused-co" {
		t.Errorf("named targets = %+v, want paused-co", targets)
	}
}
