package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bttnns/joblog/internal/store"
)

func TestAssembleRowsInsertsGaps(t *testing.T) {
	// Two fragments with a horizontal gap (10 -> 30, prevEnd=15) get a space;
	// two adjacent fragments (gap <= 1.0) do not.
	rows := [][]textFrag{
		{{X: 0, W: 15, S: "Senior"}, {X: 30, W: 20, S: "Engineer"}},
		{{X: 0, W: 10, S: "Foo"}, {X: 10.5, W: 10, S: "Bar"}},
	}
	got := assembleRows(rows)
	if !strings.Contains(got, "Senior Engineer") {
		t.Errorf("want a gap-inserted space: %q", got)
	}
	if !strings.Contains(got, "FooBar") {
		t.Errorf("want adjacent fragments joined with no space: %q", got)
	}
}

func TestTruncateRunes(t *testing.T) {
	// A multibyte string must not be sliced mid-rune into a replacement char.
	s := "café-société-naïve" // 18 runes, more bytes
	got := truncate(s, 6)
	if r := []rune(got); len(r) != 6 {
		t.Errorf("truncate to 6 runes = %q (%d runes)", got, len(r))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated value should end with ellipsis: %q", got)
	}
	if strings.Contains(got, "�") {
		t.Errorf("truncate produced a replacement char: %q", got)
	}
	if truncate("short", 40) != "short" {
		t.Errorf("no truncation when within width")
	}
}

func TestLogListRendersTable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "add", "--employer", "Acme", "--title", "SRE"); err != nil {
		t.Fatal(err)
	}
	out, err := runCapture(t, dir, "log", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"EMPLOYER", "TITLE", "Acme", "SRE"} {
		if !strings.Contains(out, want) {
			t.Errorf("log list output missing %q:\n%s", want, out)
		}
	}
}

func TestRoleListRendersTable(t *testing.T) {
	old := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = old }()

	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	writeImport(t, dir, "acme", `[{"global_id":"gh:1","title":"Senior SRE","company":"Acme","location":"Remote"}]`)
	out, err := runCapture(t, dir, "role", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"TITLE", "EMPLOYER", "Senior SRE", "Acme"} {
		if !strings.Contains(out, want) {
			t.Errorf("role list output missing %q:\n%s", want, out)
		}
	}
}

func TestReportHumanOutput(t *testing.T) {
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
	for _, emp := range []string{"Acme", "Globex", "Initech"} {
		if err := run(t, dir, "add", "--employer", emp); err != nil {
			t.Fatal(err)
		}
	}
	out, err := runCapture(t, dir, "report")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Texas", "Required activities/week:", "Compliant: true"} {
		if !strings.Contains(out, want) {
			t.Errorf("report output missing %q:\n%s", want, out)
		}
	}
}

// TestRoleImportPruneGuard verifies the destructive-import guard: an import that
// would retire most of a company's open roles is refused unless --force, and
// --no-gone imports the partial payload without retiring anything.
func TestRoleImportPruneGuard(t *testing.T) {
	old := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = old }()

	full := `[
	  {"global_id":"gh:1","title":"A","company":"Acme"},
	  {"global_id":"gh:2","title":"B","company":"Acme"},
	  {"global_id":"gh:3","title":"C","company":"Acme"},
	  {"global_id":"gh:4","title":"D","company":"Acme"},
	  {"global_id":"gh:5","title":"E","company":"Acme"}
	]`
	partial := `[{"global_id":"gh:1","title":"A","company":"Acme"}]`

	openCount := func(t *testing.T, dir string) int {
		t.Helper()
		s := &store.Store{Dir: dir}
		rs, _ := s.LoadRoles()
		n := 0
		for _, r := range rs {
			if r.Status == "open" {
				n++
			}
		}
		return n
	}

	t.Run("refuses without force", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "private")
		mustRun(t, dir, "init")
		writeImport(t, dir, "Acme", full)
		if err := importFile(t, dir, "Acme", partial); err == nil {
			t.Fatal("partial import: want guard error, got nil")
		}
		if n := openCount(t, dir); n != 5 {
			t.Errorf("after refused import: %d open roles, want 5 (unsaved)", n)
		}
	})

	t.Run("force applies", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "private")
		mustRun(t, dir, "init")
		writeImport(t, dir, "Acme", full)
		pf := writeTemp(t, partial)
		if err := run(t, dir, "role", "import", pf, "--company", "Acme", "--force"); err != nil {
			t.Fatalf("forced import: %v", err)
		}
		if n := openCount(t, dir); n != 1 {
			t.Errorf("after forced import: %d open roles, want 1 (4 retired)", n)
		}
	})

	t.Run("no-gone imports partial without retiring", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "private")
		mustRun(t, dir, "init")
		writeImport(t, dir, "Acme", full)
		pf := writeTemp(t, partial)
		if err := run(t, dir, "role", "import", pf, "--company", "Acme", "--no-gone"); err != nil {
			t.Fatalf("no-gone import: %v", err)
		}
		if n := openCount(t, dir); n != 5 {
			t.Errorf("after no-gone import: %d open roles, want 5 (none retired)", n)
		}
	})
}

// --- small test helpers ---

func mustRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	if err := run(t, dir, args...); err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
}

func writeTemp(t *testing.T, payload string) string {
	t.Helper()
	pf := filepath.Join(t.TempDir(), "payload.json")
	if err := os.WriteFile(pf, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	return pf
}

func importFile(t *testing.T, dir, company, payload string) error {
	t.Helper()
	return run(t, dir, "role", "import", writeTemp(t, payload), "--company", company)
}

func writeImport(t *testing.T, dir, company, payload string) {
	t.Helper()
	if err := importFile(t, dir, company, payload); err != nil {
		t.Fatalf("seed import: %v", err)
	}
}
