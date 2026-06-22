package state

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bttnns/joblog/internal/model"
)

// update regenerates the golden files when set: go test ./internal/state/... -update
var update = flag.Bool("update", false, "update golden files")

// allCodes is the set of states this package must implement, per DESIGN.md.
var allCodes = []string{"ca", "fl", "ga", "il", "mi", "nc", "nj", "ny", "oh", "pa", "tx", "va", "wa"}

// fixtureWeek is the shared week of activities golden files and Check tests run
// against: an application, a networking contact, a phone interview, and a
// second application to a different employer. Realistic, deterministic, dated
// within one Monday-to-Sunday week.
func fixtureWeek() []model.Entry {
	return []model.Entry{
		{
			ID: "a1", Date: "2026-06-15", Type: "applied",
			Employer: "Acme Robotics", Title: "Senior Backend Engineer",
			JobType: "Software Engineering", URL: "https://acme.example/jobs/123",
			Method: "online-portal", Status: "applied",
			Contact: "careers@acme.example", Notes: "Referred by a former colleague.",
		},
		{
			ID: "a2", Date: "2026-06-16", Type: "networking",
			Employer: "Globex Corp", Title: "", JobType: "Software Engineering",
			URL: "", Method: "linkedin", Status: "awaiting",
			Contact: "Dana Lee, Eng Manager", Notes: "Coffee chat about open roles.",
		},
		{
			ID: "a3", Date: "2026-06-17", Type: "phone-interview",
			Employer: "Initech", Title: "Platform Engineer",
			JobType: "Software Engineering", URL: "https://initech.example/careers/77",
			Method: "phone", Status: "screen",
			Contact: "Recruiter: Pat Quinn", Notes: "30-minute recruiter screen.",
		},
		{
			ID: "a4", Date: "2026-06-18", Type: "applied",
			Employer: "Hooli", Title: "Staff Engineer",
			JobType: "Software Engineering", URL: "https://hooli.example/apply/9",
			Method: "email", Status: "no-reply",
			Contact: "jobs@hooli.example", Notes: "",
		},
	}
}

func TestCodes(t *testing.T) {
	got := Codes()
	if !reflect.DeepEqual(got, allCodes) {
		t.Fatalf("Codes() = %v, want %v", got, allCodes)
	}
}

func TestAllSortedAndComplete(t *testing.T) {
	all := All()
	if len(all) != len(allCodes) {
		t.Fatalf("All() returned %d profiles, want %d", len(all), len(allCodes))
	}
	for i, p := range all {
		if p.Code() != allCodes[i] {
			t.Errorf("All()[%d].Code() = %q, want %q (not sorted or missing)", i, p.Code(), allCodes[i])
		}
	}
}

func TestGet(t *testing.T) {
	for _, code := range allCodes {
		p, ok := Get(code)
		if !ok {
			t.Errorf("Get(%q) not found", code)
			continue
		}
		if p.Code() != code {
			t.Errorf("Get(%q).Code() = %q", code, p.Code())
		}
		if p.Name() == "" || p.FormName() == "" || p.Retention() == "" || p.SourceURL() == "" {
			t.Errorf("%s: a required descriptive field is empty", code)
		}
	}
	if _, ok := Get("zz"); ok {
		t.Error("Get(\"zz\") should not be found")
	}
}

func TestCheck(t *testing.T) {
	week := fixtureWeek()
	// fixtureWeek qualifies as: 3 employer contacts (2 applied + 1 phone
	// interview) plus 1 networking activity. Distinct employers among contacts:
	// Acme, Initech, Hooli (3).
	tests := []struct {
		name   string
		code   string
		min    int
		wantN  int
		wantOK bool
	}{
		{"tx default min 3 met", "tx", 3, 4, true},
		{"tx override min 6 unmet", "tx", 6, 4, false},
		{"ca min 0 always ok", "ca", 0, 4, true},
		{"il min 0 always ok", "il", 0, 4, true},
		{"oh min 2 met", "oh", 2, 4, true},
		{"ga new contacts min 3 met", "ga", 3, 3, true}, // only the 3 employer contacts count
		{"ga new contacts min 4 unmet", "ga", 4, 3, false},
		{"pa compound met", "pa", 3, 4, true}, // 3 apps + 1 networking
		{"wa broad min 3 met", "wa", 3, 4, true},
		{"va two employers min 2 met", "va", 2, 3, true}, // 3 contacts, 3 distinct employers
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := Get(tt.code)
			if !ok {
				t.Fatalf("Get(%q) not found", tt.code)
			}
			n, gotOK := p.Check(week, tt.min)
			if n != tt.wantN || gotOK != tt.wantOK {
				t.Errorf("%s.Check(min=%d) = (%d, %v), want (%d, %v)",
					tt.code, tt.min, n, gotOK, tt.wantN, tt.wantOK)
			}
		})
	}
}

// TestCheckPACompound exercises PA's compound rule directly: 2 applications AND
// 1 other activity are both required.
func TestCheckPACompound(t *testing.T) {
	pa, _ := Get("pa")
	tests := []struct {
		name   string
		week   []model.Entry
		wantN  int
		wantOK bool
	}{
		{
			name: "2 apps + 1 networking meets",
			week: []model.Entry{
				{Type: "applied", Employer: "A"},
				{Type: "applied", Employer: "B"},
				{Type: "networking", Employer: "C"},
			},
			wantN: 3, wantOK: true,
		},
		{
			name: "3 apps but no other activity fails compound",
			week: []model.Entry{
				{Type: "applied", Employer: "A"},
				{Type: "applied", Employer: "B"},
				{Type: "applied", Employer: "C"},
			},
			wantN: 3, wantOK: false,
		},
		{
			name: "1 app + 2 networking fails compound",
			week: []model.Entry{
				{Type: "applied", Employer: "A"},
				{Type: "networking", Employer: "B"},
				{Type: "networking", Employer: "C"},
			},
			wantN: 3, wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// min 0 here so we test only the compound rule, not a numeric floor.
			n, ok := pa.Check(tt.week, 0)
			if n != tt.wantN || ok != tt.wantOK {
				t.Errorf("pa.Check = (%d, %v), want (%d, %v)", n, ok, tt.wantN, tt.wantOK)
			}
		})
	}
}

// TestCheckDistinctness covers the states that require N DIFFERENT employers
// (GA, VA) or activities on N DIFFERENT days (NY): repeated contacts to one
// company, or several activities on one day, must not over-credit the week.
func TestCheckDistinctness(t *testing.T) {
	// Three contacts that all resolve to the same company slug.
	sameCompany := []model.Entry{
		{Type: "applied", Employer: "Fastly", Company: "fastly", Date: "2026-06-15"},
		{Type: "applied", Employer: "Fastly Inc.", Company: "fastly", Date: "2026-06-15"},
		{Type: "phone-interview", Employer: "Fastly", Company: "fastly", Date: "2026-06-16"},
	}
	ga, _ := Get("ga")
	if n, ok := ga.Check(sameCompany, 3); ok {
		t.Errorf("ga.Check same-company = (%d, true), want unmet (1 distinct employer)", n)
	}
	va, _ := Get("va")
	if _, ok := va.Check(sameCompany, 2); ok {
		t.Error("va.Check same-company (min 2) should be unmet (1 distinct employer)")
	}
	// Distinct contacts to distinct companies meet the bar.
	threeCos := []model.Entry{
		{Type: "applied", Company: "a", Date: "2026-06-15"},
		{Type: "applied", Company: "b", Date: "2026-06-16"},
		{Type: "phone-interview", Company: "c", Date: "2026-06-17"},
	}
	if _, ok := ga.Check(threeCos, 3); !ok {
		t.Error("ga.Check three distinct companies (min 3) should be met")
	}

	ny, _ := Get("ny")
	// Three activities on two days: below NY's different-days bar.
	twoDays := []model.Entry{
		{Type: "applied", Company: "a", Date: "2026-06-15"},
		{Type: "applied", Company: "b", Date: "2026-06-15"},
		{Type: "networking", Company: "c", Date: "2026-06-16"},
	}
	if _, ok := ny.Check(twoDays, 3); ok {
		t.Error("ny.Check 3 activities on 2 days (min 3) should be unmet")
	}
	// Spread the same three over three days: met.
	threeDays := []model.Entry{
		{Type: "applied", Company: "a", Date: "2026-06-15"},
		{Type: "applied", Company: "b", Date: "2026-06-16"},
		{Type: "networking", Company: "c", Date: "2026-06-17"},
	}
	if _, ok := ny.Check(threeDays, 3); !ok {
		t.Error("ny.Check 3 activities on 3 days (min 3) should be met")
	}
}

func TestRenderGolden(t *testing.T) {
	week := fixtureWeek()
	for _, p := range All() {
		p := p
		t.Run(p.Code(), func(t *testing.T) {
			got := p.Render(week)
			golden := filepath.Join("testdata", p.Code()+".golden")
			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden (run with -update to create): %v", err)
			}
			if got != string(want) {
				t.Errorf("%s Render mismatch (run with -update to regenerate)\n--- got ---\n%s\n--- want ---\n%s",
					p.Code(), got, want)
			}
		})
	}
}
