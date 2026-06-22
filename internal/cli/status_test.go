package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestStatusChecks is table-driven over a couple of store states (empty vs
// partially set up) and asserts each check reports done/not-done correctly and
// that the --json shape carries name/ok/detail/next per check.
func TestStatusChecks(t *testing.T) {
	old := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = old }()

	cases := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  map[string]bool // check name -> ok
	}{
		{
			name:  "empty store",
			setup: func(t *testing.T, dir string) {},
			want: map[string]bool{
				"resume":    false,
				"state":     false,
				"profile":   false,
				"companies": false,
				"roles":     false,
				"this week": false,
			},
		},
		{
			name: "partially set up",
			setup: func(t *testing.T, dir string) {
				// State configured, a company tracked, and a meeting (3) week of
				// applications so the weekly check passes; resume, profile, and
				// roles stay unset.
				if err := run(t, dir, "config", "set", "state", "tx"); err != nil {
					t.Fatal(err)
				}
				if err := run(t, dir, "company", "add", "--name", "acme", "--ats", "ashby", "--slug", "acme"); err != nil {
					t.Fatal(err)
				}
				for _, emp := range []string{"Acme", "Globex", "Initech"} {
					if err := run(t, dir, "add", "--employer", emp); err != nil {
						t.Fatal(err)
					}
				}
			},
			want: map[string]bool{
				"resume":    false,
				"state":     true,
				"profile":   false,
				"companies": true,
				"roles":     false,
				"this week": true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "private")
			tc.setup(t, dir)

			out, err := runCapture(t, dir, "--json", "status")
			if err != nil {
				t.Fatalf("status: %v", err)
			}
			var checks []statusCheck
			if e := json.Unmarshal([]byte(out), &checks); e != nil {
				t.Fatalf("status --json is not a check array: %v (%q)", e, out)
			}
			got := map[string]bool{}
			for _, c := range checks {
				got[c.Name] = c.OK
				// A not-done check must carry a next command; a done one need not.
				if !c.OK && c.Next == "" {
					t.Errorf("check %q is not done but has no next command", c.Name)
				}
			}
			for name, wantOK := range tc.want {
				gotOK, present := got[name]
				if !present {
					t.Errorf("missing check %q", name)
					continue
				}
				if gotOK != wantOK {
					t.Errorf("check %q ok = %v, want %v", name, gotOK, wantOK)
				}
			}
		})
	}
}

// TestProfileFilled covers the profile heuristic: an all-TODO scaffold is not
// filled, but a profile with real prose is.
func TestProfileFilled(t *testing.T) {
	dir := t.TempDir()

	missing := filepath.Join(dir, "missing.md")
	if profileFilled(missing) {
		t.Error("a missing profile.md should not count as filled")
	}

	scaffold := filepath.Join(dir, "scaffold.md")
	if err := os.WriteFile(scaffold, []byte("# Profile\n\n## Summary\nTODO: a short summary.\n\n## Salary\n- Floor: TODO\n- Target: TODO\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if profileFilled(scaffold) {
		t.Error("an all-TODO scaffold should not count as filled")
	}

	filled := filepath.Join(dir, "filled.md")
	if err := os.WriteFile(filled, []byte("# Profile\n\n## Summary\nSRE with 8 years of distributed systems experience.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !profileFilled(filled) {
		t.Error("a profile with real prose should count as filled")
	}
}

// TestImplicitInit verifies a write command scaffolds the data dir without a
// prior `jl init`, and that bare `jl` (no subcommand) runs the status map.
func TestImplicitInit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")

	// No init: a write command should create the tree on its own.
	if err := run(t, dir, "config", "set", "state", "tx"); err != nil {
		t.Fatalf("config set without init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "data")); err != nil {
		t.Errorf("data dir not scaffolded implicitly: %v", err)
	}

	// Bare `jl` runs status (read-only); it should not error on a fresh dir.
	out, err := runCapture(t, dir, "--json")
	if err != nil {
		t.Fatalf("bare jl: %v", err)
	}
	var checks []statusCheck
	if e := json.Unmarshal([]byte(out), &checks); e != nil {
		t.Fatalf("bare jl --json is not a status array: %v (%q)", e, out)
	}
	if len(checks) == 0 {
		t.Error("bare jl produced no status checks")
	}
}
