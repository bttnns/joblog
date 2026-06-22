package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bttnns/joblog/internal/store"
)

// TestScraperArgv covers the {ats}/{slug} template substitution and the default
// template, the contract jl fetch relies on to build the scraper command.
func TestScraperArgv(t *testing.T) {
	cases := []struct {
		name      string
		template  string
		ats, slug string
		want      []string
	}{
		{
			name:     "default template",
			template: store.DefaultScraper,
			ats:      "ashby",
			slug:     "spacex",
			want:     []string{"jobhive", "scrape", "ashby", "spacex", "--format", "json"},
		},
		{
			name:     "custom order and flags",
			template: "myscraper --ats {ats} --company {slug} -o json",
			ats:      "greenhouse",
			slug:     "acme",
			want:     []string{"myscraper", "--ats", "greenhouse", "--company", "acme", "-o", "json"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scraperArgv(tc.template, tc.ats, tc.slug)
			if err != nil {
				t.Fatalf("scraperArgv: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("argv = %v, want %v", got, tc.want)
			}
		})
	}

	if _, err := scraperArgv("   ", "a", "b"); err == nil {
		t.Error("empty template: want error")
	}
}

// writeStubScraper writes a shell script that ignores its args and echoes the
// given JSON payload to stdout, then returns a scraper template that invokes it.
// It is the test's stand-in for jobhive, so the fetch path is exercised without
// any real scraper installed or any network call.
func writeStubScraper(t *testing.T, payload string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub scraper is a shell script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "stub-scraper")
	body := "#!/bin/sh\ncat <<'JSON'\n" + payload + "\nJSON\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script + " {ats} {slug}"
}

// TestFetch exercises the happy path end to end without depending on jobhive: a
// stub scraper feeds a fixture payload, fetch scrapes + imports it, and the roles
// land in the index. It also covers the skip path for a custom-ATS company and
// the per-company JSON result shape.
func TestFetch(t *testing.T) {
	old := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = old }()

	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}

	payload := `[
	  {"global_id":"ashby:1","title":"Senior SRE","company":"SpaceX","location":"Hawthorne, CA","url":"https://x/1","salary_summary":"$200k"},
	  {"global_id":"ashby:2","title":"Backend Engineer","company":"SpaceX","location":"Remote","url":"https://x/2"},
	  {"global_id":"ashby:3","title":"Data Engineer","company":"SpaceX","location":"Austin, TX","url":"https://x/3"}
	]`
	template := writeStubScraper(t, payload)
	if err := run(t, dir, "config", "set", "scraper", template); err != nil {
		t.Fatalf("config set scraper: %v", err)
	}

	if err := run(t, dir, "company", "add", "--name", "spacex", "--ats", "ashby", "--slug", "spacex"); err != nil {
		t.Fatalf("company add: %v", err)
	}
	// A custom-ATS company must be skipped, not fetched.
	if err := run(t, dir, "company", "add", "--name", "weird", "--ats", "custom"); err != nil {
		t.Fatalf("company add custom: %v", err)
	}

	out, err := runCapture(t, dir, "--json", "fetch")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	var results []fetchResult
	if e := json.Unmarshal([]byte(out), &results); e != nil {
		t.Fatalf("fetch --json not an array: %v (%q)", e, out)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2 (one fetched, one skipped)", len(results))
	}

	var spacex, weird *fetchResult
	for i := range results {
		switch results[i].Name {
		case "spacex":
			spacex = &results[i]
		case "weird":
			weird = &results[i]
		}
	}
	if spacex == nil || weird == nil {
		t.Fatalf("missing a company in results: %+v", results)
	}
	if spacex.New != 3 || spacex.Changed != 0 || spacex.Gone != 0 {
		t.Errorf("spacex delta = %+v, want 3 new / 0 changed / 0 gone", spacex)
	}
	if !weird.Skipped {
		t.Errorf("custom-ATS company should be skipped, got %+v", weird)
	}

	s := &store.Store{Dir: dir}
	rs, err := s.LoadRoles()
	if err != nil || len(rs) != 3 {
		t.Fatalf("roles imported = %d, %v; want 3", len(rs), err)
	}

	// A second fetch with one role removed marks it gone (gone-marking flows
	// through the shared import core).
	smaller := `[
	  {"global_id":"ashby:1","title":"Senior SRE","company":"SpaceX","location":"Hawthorne, CA","url":"https://x/1","salary_summary":"$200k"},
	  {"global_id":"ashby:2","title":"Backend Engineer","company":"SpaceX","location":"Remote","url":"https://x/2"}
	]`
	template2 := writeStubScraper(t, smaller)
	if err := run(t, dir, "config", "set", "scraper", template2); err != nil {
		t.Fatal(err)
	}
	out, err = runCapture(t, dir, "--json", "fetch", "spacex")
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	results = nil
	if e := json.Unmarshal([]byte(out), &results); e != nil {
		t.Fatalf("second fetch --json: %v", e)
	}
	if len(results) != 1 || results[0].Gone != 1 {
		t.Errorf("second fetch results = %+v, want 1 company with 1 gone", results)
	}

	// An unknown company name is an error.
	if err := run(t, dir, "fetch", "nope"); err == nil {
		t.Error("fetch of unknown company: want error")
	}
}

// TestFetchScraperFailureContinues verifies that one company's scrape failure
// does not abort the rest of the run: a missing-binary scraper still lets fetch
// finish, reporting the error per company.
func TestFetchScraperFailureContinues(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := run(t, dir, "init"); err != nil {
		t.Fatal(err)
	}
	if err := run(t, dir, "config", "set", "scraper", "jl-no-such-scraper-binary {ats} {slug}"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta"} {
		if err := run(t, dir, "company", "add", "--name", name, "--ats", "ashby", "--slug", name); err != nil {
			t.Fatal(err)
		}
	}
	out, err := runCapture(t, dir, "--json", "fetch")
	if err != nil {
		t.Fatalf("fetch should not return an error when individual scrapes fail: %v", err)
	}
	var results []fetchResult
	if e := json.Unmarshal([]byte(out), &results); e != nil {
		t.Fatalf("fetch --json: %v (%q)", e, out)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2 (both attempted despite failure)", len(results))
	}
	for _, r := range results {
		if r.Error == "" {
			t.Errorf("%s: want a scrape error recorded, got %+v", r.Name, r)
		}
		if !strings.Contains(r.Error, "install a scraper") {
			t.Errorf("%s: missing-binary error should carry the install hint, got %q", r.Name, r.Error)
		}
	}
}
