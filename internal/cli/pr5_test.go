package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bttnns/joblog/internal/store"
)

// TestCompanySearch exercises `jl company search` against the embedded catalog:
// the table output, --json, the --ats and --limit flags, and the tracked marker.
// It anchors on stripe (greenhouse, unambiguous in the snapshot).
func TestCompanySearch(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}

	// Plain table output names the company and its derived URL.
	out, err := runCapture(t, dir, "company", "search", "stripe")
	if err != nil {
		t.Fatalf("company search: %v", err)
	}
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "Stripe") {
		t.Errorf("search table missing header or Stripe: %q", out)
	}
	if !strings.Contains(out, "boards.greenhouse.io/stripe") {
		t.Errorf("search table missing derived URL: %q", out)
	}

	// --json: a structured, ranked list with the expected fields. Stripe should
	// rank first for an exact-name query.
	out, err = runCapture(t, dir, "--json", "company", "search", "stripe")
	if err != nil {
		t.Fatalf("company search --json: %v", err)
	}
	var rows []searchResult
	if e := json.Unmarshal([]byte(out), &rows); e != nil {
		t.Fatalf("search not JSON: %v (%q)", e, out)
	}
	if len(rows) == 0 {
		t.Fatal("search --json returned no rows")
	}
	if rows[0].Name != "Stripe" || rows[0].ATS != "greenhouse" || rows[0].Slug != "stripe" {
		t.Errorf("top hit = %+v, want Stripe/greenhouse/stripe", rows[0])
	}
	if rows[0].URL != "https://boards.greenhouse.io/stripe" {
		t.Errorf("top hit URL = %q, want derived greenhouse URL", rows[0].URL)
	}
	if rows[0].Tracked != "untracked" {
		t.Errorf("untracked company marked %q, want untracked", rows[0].Tracked)
	}

	// --limit caps the result count.
	out, _ = runCapture(t, dir, "--json", "company", "search", "a", "--limit", "3")
	rows = nil
	_ = json.Unmarshal([]byte(out), &rows)
	if len(rows) != 3 {
		t.Errorf("--limit 3 returned %d rows, want 3", len(rows))
	}

	// --ats restricts the ATS of every hit.
	out, _ = runCapture(t, dir, "--json", "company", "search", "a", "--ats", "lever", "--limit", "10")
	rows = nil
	_ = json.Unmarshal([]byte(out), &rows)
	if len(rows) == 0 {
		t.Fatal("--ats lever returned nothing")
	}
	for _, r := range rows {
		if r.ATS != "lever" {
			t.Errorf("--ats lever hit has ATS %q", r.ATS)
		}
	}
}

// TestCompanySearchTrackedMarker verifies search reflects a company you already
// track, including its paused status.
func TestCompanySearchTrackedMarker(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	// Track stripe (by its catalog slug), then pause it.
	if err := run(t, dir, "company", "add", "stripe"); err != nil {
		t.Fatalf("add stripe by slug: %v", err)
	}
	if err := run(t, dir, "company", "set", "Stripe", "paused"); err != nil {
		t.Fatalf("set paused: %v", err)
	}

	out, err := runCapture(t, dir, "--json", "company", "search", "stripe")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	var rows []searchResult
	if e := json.Unmarshal([]byte(out), &rows); e != nil {
		t.Fatal(e)
	}
	if rows[0].Tracked != store.StatusPaused {
		t.Errorf("tracked marker = %q, want paused", rows[0].Tracked)
	}
}

// TestCompanyAddBySlug covers resolving a positional arg as a catalog slug,
// including the ambiguous-slug error and the --ats disambiguation.
func TestCompanyAddBySlug(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{Dir: dir}

	// stripe is unambiguous: resolves to greenhouse with the derived URL.
	if err := run(t, dir, "company", "add", "stripe"); err != nil {
		t.Fatalf("add by slug: %v", err)
	}
	cs, _ := s.LoadCompanies()
	if len(cs) != 1 {
		t.Fatalf("companies = %d, want 1: %+v", len(cs), cs)
	}
	c := cs[0]
	if c.Name != "Stripe" || c.ATS != "greenhouse" || c.Slug != "stripe" {
		t.Errorf("added = %+v, want Stripe/greenhouse/stripe", c)
	}
	if c.CareersURL != "https://boards.greenhouse.io/stripe" {
		t.Errorf("careers URL = %q, want derived", c.CareersURL)
	}

	// verkada is in the catalog under two ATSes: a bare slug is ambiguous.
	if err := run(t, dir, "company", "add", "verkada"); err == nil {
		t.Error("ambiguous slug add: want error asking for --ats")
	}
	// --ats disambiguates it.
	if err := run(t, dir, "company", "add", "verkada", "--ats", "ashby"); err != nil {
		t.Fatalf("add ambiguous slug with --ats: %v", err)
	}
	cs, _ = s.LoadCompanies()
	var sawVerkada bool
	for _, c := range cs {
		if c.Slug == "verkada" {
			sawVerkada = true
			if c.ATS != "ashby" {
				t.Errorf("verkada ATS = %q, want ashby", c.ATS)
			}
		}
	}
	if !sawVerkada {
		t.Error("verkada not added")
	}

	// A slug not in the catalog is a clear error (and not mistaken for a URL).
	if err := run(t, dir, "company", "add", "definitelynotacompanyslug123"); err == nil {
		t.Error("unknown slug add: want error")
	}
}
