package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bttnns/joblog/internal/model"
)

func TestResolvePrecedence(t *testing.T) {
	// Override wins over everything.
	t.Setenv("JOBLOG_HOME", "/tmp/jh")
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg")
	got, err := Resolve("/tmp/override")
	if err != nil || got != "/tmp/override" {
		t.Fatalf("override: got %q, %v", got, err)
	}

	// JOBLOG_HOME wins over XDG.
	got, _ = Resolve("")
	if got != "/tmp/jh" {
		t.Fatalf("JOBLOG_HOME: got %q", got)
	}

	// XDG when JOBLOG_HOME unset.
	t.Setenv("JOBLOG_HOME", "")
	got, _ = Resolve("")
	if got != filepath.Join("/tmp/xdg", "joblog") {
		t.Fatalf("XDG: got %q", got)
	}
}

func TestInitAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if !s.Initialized() {
		t.Fatal("Initialized() = false after Init")
	}
	// Scaffolded files exist.
	for _, p := range []string{"data/log.json", "data/roles.json", "data/companies.yaml", "config.yaml", "README.md", "profile.md", "accomplishments.md"} {
		if _, err := os.Stat(s.Path(p)); err != nil {
			t.Errorf("missing scaffolded %s: %v", p, err)
		}
	}

	// Log round trip.
	want := []model.Entry{{ID: "abc12345", Date: "2026-03-16", Type: "applied", Employer: "Acme", Status: "applied"}}
	if err := s.SaveLog(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadLog()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "abc12345" || got[0].Employer != "Acme" {
		t.Errorf("log round trip = %+v", got)
	}

	// Config round trip.
	if err := s.SaveConfig(Config{State: "tx", Min: 3, ResumePath: "resume/resume.pdf"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.LoadConfig()
	if err != nil || cfg.State != "tx" || cfg.Min != 3 {
		t.Errorf("config round trip = %+v, %v", cfg, err)
	}

	// Companies round trip.
	if err := s.SaveCompanies([]Company{{Name: "acme", ATS: "greenhouse", Slug: "acme"}}); err != nil {
		t.Fatal(err)
	}
	cs, err := s.LoadCompanies()
	if err != nil || len(cs) != 1 || cs[0].ATS != "greenhouse" {
		t.Errorf("companies round trip = %+v, %v", cs, err)
	}
}

// TestLoadCompaniesMigratesLegacyTargets verifies that a pre-rename targets.yaml
// is read when companies.yaml is absent, so existing data keeps working.
func TestLoadCompaniesMigratesLegacyTargets(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir}
	legacy := "targets:\n  - name: fastly\n    ats: greenhouse\n    slug: fastly\n    careers-url: https://x\n"
	if err := writeAtomic(s.legacyTargetsPath(), []byte(legacy)); err != nil {
		t.Fatal(err)
	}
	cs, err := s.LoadCompanies()
	if err != nil || len(cs) != 1 || cs[0].Name != "fastly" || cs[0].ATS != "greenhouse" {
		t.Fatalf("legacy migration = %+v, %v", cs, err)
	}
}

func TestInitIdempotent(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	// Put data in, re-init, ensure not clobbered.
	if err := s.SaveLog([]model.Entry{{ID: "keepme01", Date: "2026-03-16", Type: "applied", Status: "applied"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	got, _ := s.LoadLog()
	if len(got) != 1 || got[0].ID != "keepme01" {
		t.Errorf("Init clobbered data: %+v", got)
	}
}

func TestNewID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewID()
		if len(id) != 8 {
			t.Fatalf("id %q len = %d, want 8", id, len(id))
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

// TestLoadLegacyBareArray verifies a pre-versioning log.json/roles.json (a bare
// JSON array, schema 0) still loads, so existing data keeps working.
func TestLoadLegacyBareArray(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	legacyLog := `[{"id":"abc12345","date":"2026-03-16","type":"applied","employer":"Acme","status":"applied"}]`
	if err := writeAtomic(s.logPath(), []byte(legacyLog)); err != nil {
		t.Fatal(err)
	}
	log, err := s.LoadLog()
	if err != nil || len(log) != 1 || log[0].Employer != "Acme" {
		t.Fatalf("legacy log load = %+v, %v", log, err)
	}
	legacyRoles := `[{"global_id":"gh:1","title":"SRE","employer":"Acme","status":"open"}]`
	if err := writeAtomic(s.rolesPath(), []byte(legacyRoles)); err != nil {
		t.Fatal(err)
	}
	roles, err := s.LoadRoles()
	if err != nil || len(roles) != 1 || roles[0].GlobalID != "gh:1" {
		t.Fatalf("legacy roles load = %+v, %v", roles, err)
	}
}

// TestSaveWritesVersionedEnvelope verifies a save round-trips through the
// versioned envelope (schema field present) rather than a bare array.
func TestSaveWritesVersionedEnvelope(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := s.SaveLog([]model.Entry{{ID: "x", Date: "2026-03-16", Type: "applied", Status: "applied"}}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(s.logPath())
	if err != nil {
		t.Fatal(err)
	}
	if looksLikeArray(b) {
		t.Errorf("SaveLog wrote a bare array, want a versioned envelope: %s", b)
	}
	var lf logFile
	if err := json.Unmarshal(b, &lf); err != nil || lf.Schema != schemaVersion || len(lf.Entries) != 1 {
		t.Errorf("envelope = %+v, %v", lf, err)
	}
}

// TestRejectsFutureSchema verifies a file from a newer jl is refused rather than
// silently misread as empty.
func TestRejectsFutureSchema(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	future := `{"schema":999,"entries":[]}`
	if err := writeAtomic(s.logPath(), []byte(future)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadLog(); err == nil {
		t.Error("LoadLog on a future schema: want error, got nil")
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	log, err := s.LoadLog()
	if err != nil || len(log) != 0 {
		t.Errorf("LoadLog on empty dir = %+v, %v", log, err)
	}
	roles, err := s.LoadRoles()
	if err != nil || len(roles) != 0 {
		t.Errorf("LoadRoles on empty dir = %+v, %v", roles, err)
	}
}
