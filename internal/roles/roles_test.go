package roles

import (
	"testing"
	"time"

	"github.com/bttnns/joblog/internal/model"
)

// firstPayload is a jobhive-style array with four roles exercising the schema
// edge cases: null salary_min/max with a salary_summary (eng), epoch-millis
// posted_at (design), null is_remote with a "Remote" location (support), and an
// explicit is_remote=false (ops). Unknown fields (ats_type, raw) are present to
// confirm they are ignored.
const firstPayload = `[
  {
    "global_id": "ashby:uuid-eng-1:extra",
    "url": "https://jobs.example.com/eng",
    "title": "Senior Engineer",
    "company": "acme-corp",
    "ats_type": "ashby",
    "ats_id": "uuid-eng-1:extra",
    "location": "San Francisco, CA",
    "is_remote": false,
    "salary_summary": "$180k - $220k",
    "salary_min": null,
    "salary_max": null,
    "description": "Build distributed systems.",
    "posted_at": "2026-06-01T00:00:00Z",
    "fetched_at": null,
    "raw": {"anything": 1}
  },
  {
    "global_id": "ashby:design-2",
    "url": "https://jobs.example.com/design",
    "title": "Product Designer",
    "company": "acme-corp",
    "location": "New York, NY",
    "is_remote": false,
    "salary_min": 120000,
    "salary_max": 150000,
    "description": "Design things.",
    "posted_at": 1717200000000,
    "fetched_at": 1717286400000
  },
  {
    "global_id": "ashby:support-3",
    "url": "https://jobs.example.com/support",
    "title": "Support Specialist",
    "company": "acme-corp",
    "location": "Remote - US",
    "is_remote": null,
    "salary_summary": "",
    "description": "Help customers."
  },
  {
    "global_id": "",
    "url": "https://jobs.example.com/ops",
    "title": "Operations Lead",
    "company": "acme-corp",
    "location": "Austin, TX",
    "salary_max": 95000.5,
    "description": "Run ops."
  }
]`

func mustImport(t *testing.T, existing []model.Role, payload, company string, now time.Time) ([]model.Role, ImportResult) {
	t.Helper()
	roles, res, err := Import(existing, []byte(payload), company, now)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return roles, res
}

func TestImportEmptyIndexAllNew(t *testing.T) {
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	roles, res := mustImport(t, nil, firstPayload, "acme-corp", now)

	if n, c, g := res.Counts(); n != 4 || c != 0 || g != 0 {
		t.Fatalf("counts = (%d,%d,%d), want (4,0,0)", n, c, g)
	}
	if len(roles) != 4 {
		t.Fatalf("len(roles) = %d, want 4", len(roles))
	}

	byID := map[string]model.Role{}
	for _, r := range roles {
		byID[r.GlobalID] = r
	}

	eng := byID["ashby:uuid-eng-1:extra"]
	if eng.Salary != "$180k - $220k" {
		t.Errorf("eng salary = %q, want summary", eng.Salary)
	}
	if eng.FirstSeen != "2026-06-10" || eng.LastSeen != "2026-06-10" {
		t.Errorf("eng seen = (%q,%q), want 2026-06-10", eng.FirstSeen, eng.LastSeen)
	}
	if eng.Status != model.RoleOpen {
		t.Errorf("eng status = %q, want open", eng.Status)
	}
	if eng.Remote {
		t.Errorf("eng remote = true, want false (explicit is_remote=false)")
	}

	design := byID["ashby:design-2"]
	if design.Salary != "$120000 - $150000" {
		t.Errorf("design salary = %q, want formatted range", design.Salary)
	}

	support := byID["ashby:support-3"]
	if !support.Remote {
		t.Errorf("support remote = false, want true (inferred from location)")
	}
	if support.Salary != "" {
		t.Errorf("support salary = %q, want empty", support.Salary)
	}

	// The role with an empty global_id falls back to its url as the key.
	ops := byID["https://jobs.example.com/ops"]
	if ops.Title != "Operations Lead" {
		t.Errorf("ops fallback key not found; got title %q", ops.Title)
	}
	if ops.Salary != "up to $95000.50" {
		t.Errorf("ops salary = %q, want up to figure", ops.Salary)
	}

	if res.Company != "acme-corp" {
		t.Errorf("result company = %q", res.Company)
	}
	if _, err := time.Parse(time.RFC3339, res.At); err != nil {
		t.Errorf("result.At %q not RFC3339: %v", res.At, err)
	}
}

func TestImportSecondPassChangedAndGone(t *testing.T) {
	now1 := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	roles, _ := mustImport(t, nil, firstPayload, "acme-corp", now1)

	// Second payload: drop the design role (=> gone), change the eng title
	// (=> changed), keep support unchanged, drop ops too (=> gone).
	const second = `[
      {
        "global_id": "ashby:uuid-eng-1:extra",
        "url": "https://jobs.example.com/eng",
        "title": "Staff Engineer",
        "company": "acme-corp",
        "location": "San Francisco, CA",
        "is_remote": false,
        "salary_summary": "$180k - $220k",
        "description": "Build distributed systems."
      },
      {
        "global_id": "ashby:support-3",
        "url": "https://jobs.example.com/support",
        "title": "Support Specialist",
        "company": "acme-corp",
        "location": "Remote - US",
        "description": "Help customers."
      }
    ]`

	now2 := time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)
	roles2, res := mustImport(t, roles, second, "acme-corp", now2)

	if n, c, g := res.Counts(); n != 0 || c != 1 || g != 2 {
		t.Fatalf("counts = (%d,%d,%d), want (0,1,2)", n, c, g)
	}
	if len(roles2) != 4 {
		t.Fatalf("len(roles2) = %d, want 4 (no rows dropped)", len(roles2))
	}

	byID := map[string]model.Role{}
	for _, r := range roles2 {
		byID[r.GlobalID] = r
	}

	eng := byID["ashby:uuid-eng-1:extra"]
	if eng.Title != "Staff Engineer" {
		t.Errorf("eng title = %q, want updated", eng.Title)
	}
	if eng.FirstSeen != "2026-06-10" {
		t.Errorf("eng FirstSeen = %q, want preserved 2026-06-10", eng.FirstSeen)
	}
	if eng.LastSeen != "2026-06-17" {
		t.Errorf("eng LastSeen = %q, want bumped 2026-06-17", eng.LastSeen)
	}
	if eng.Status != model.RoleOpen {
		t.Errorf("eng status = %q, want open", eng.Status)
	}

	if byID["ashby:design-2"].Status != model.RoleGone {
		t.Errorf("design status = %q, want gone", byID["ashby:design-2"].Status)
	}
	if byID["https://jobs.example.com/ops"].Status != model.RoleGone {
		t.Errorf("ops status = %q, want gone", byID["https://jobs.example.com/ops"].Status)
	}

	support := byID["ashby:support-3"]
	if support.Status != model.RoleOpen {
		t.Errorf("support status = %q, want open", support.Status)
	}
	if support.LastSeen != "2026-06-17" {
		t.Errorf("support LastSeen = %q, want bumped", support.LastSeen)
	}
}

func TestImportRevivesGoneRole(t *testing.T) {
	gone := []model.Role{{
		GlobalID: "ashby:design-2", Title: "Product Designer", Employer: "acme-corp",
		FirstSeen: "2026-06-01", LastSeen: "2026-06-05", Status: model.RoleGone,
	}}
	now := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	const payload = `[{"global_id":"ashby:design-2","title":"Product Designer","company":"acme-corp"}]`
	roles, res := mustImport(t, gone, payload, "acme-corp", now)
	if roles[0].Status != model.RoleOpen {
		t.Errorf("status = %q, want open (revived)", roles[0].Status)
	}
	if n, _, g := res.Counts(); n != 0 || g != 0 {
		t.Errorf("counts new=%d gone=%d, want 0/0", n, g)
	}
}

func TestImportGoneScopedToCompany(t *testing.T) {
	other := []model.Role{{
		GlobalID: "lever:beta-1", Title: "PM", Employer: "beta-inc",
		FirstSeen: "2026-06-01", LastSeen: "2026-06-01", Status: model.RoleOpen,
	}}
	now := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	const payload = `[{"global_id":"ashby:new-1","title":"Eng","company":"acme-corp"}]`
	roles, res := mustImport(t, other, payload, "acme-corp", now)
	for _, r := range roles {
		if r.GlobalID == "lever:beta-1" && r.Status != model.RoleOpen {
			t.Errorf("other company role marked %q, want untouched open", r.Status)
		}
	}
	if _, _, g := res.Counts(); g != 0 {
		t.Errorf("gone = %d, want 0 (different company)", g)
	}
}

// TestImportGoneMarkingUsesCompanySlugNotEmployer is the regression test for the
// bug where gone-marking compared the free-text payload employer against the
// --company import label. When they differ ("Fastly Inc." vs "fastly"), the old
// code marked nothing gone and stale roles accumulated forever. Gone-marking now
// scopes by the canonical company slug stamped at import, so the dropped role is
// correctly retired even though its Employer never equals the import label.
func TestImportGoneMarkingUsesCompanySlugNotEmployer(t *testing.T) {
	now1 := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	const first = `[
      {"global_id":"gh:1","title":"SRE","company":"Fastly Inc.","url":"https://x/1"},
      {"global_id":"gh:2","title":"PM","company":"Fastly Inc.","url":"https://x/2"}
    ]`
	// Import under a slug-style label that does NOT equal the payload company.
	roles1, _ := mustImport(t, nil, first, "fastly", now1)
	for _, r := range roles1 {
		if r.Employer != "Fastly Inc." {
			t.Fatalf("employer = %q, want display name from payload", r.Employer)
		}
		if r.Company != "fastly" {
			t.Fatalf("company slug = %q, want fastly", r.Company)
		}
	}

	// Second pass drops gh:2; it must be marked gone via the slug match.
	const second = `[{"global_id":"gh:1","title":"SRE","company":"Fastly Inc.","url":"https://x/1"}]`
	now2 := time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)
	roles2, res := mustImport(t, roles1, second, "fastly", now2)

	if _, _, g := res.Counts(); g != 1 {
		t.Fatalf("gone = %d, want 1 (dropped role retired despite employer != label)", g)
	}
	byID := map[string]model.Role{}
	for _, r := range roles2 {
		byID[r.GlobalID] = r
	}
	if byID["gh:2"].Status != model.RoleGone {
		t.Errorf("gh:2 status = %q, want gone", byID["gh:2"].Status)
	}
	if byID["gh:1"].Status != model.RoleOpen {
		t.Errorf("gh:1 status = %q, want open", byID["gh:1"].Status)
	}
}

// TestNormalizeURLKeyStable verifies a URL-keyed role keeps its identity when the
// same posting is re-scraped with tracking params, a trailing slash, or a
// differently-cased host, instead of churning new-then-gone.
func TestNormalizeURLKeyStable(t *testing.T) {
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	r1, _ := mustImport(t, nil, `[{"title":"Eng","company":"acme","url":"https://jobs.acme.com/eng"}]`, "acme", now)
	if len(r1) != 1 {
		t.Fatalf("want 1 role, got %d", len(r1))
	}
	second := `[{"title":"Eng","company":"acme","url":"https://JOBS.acme.com/eng/?utm=x#frag"}]`
	r2, res := mustImport(t, r1, second, "acme", now.AddDate(0, 0, 7))
	if len(r2) != 1 {
		t.Fatalf("want still 1 role (stable URL key), got %d", len(r2))
	}
	if n, _, g := res.Counts(); n != 0 || g != 0 {
		t.Errorf("counts new=%d gone=%d, want 0/0 (same role)", n, g)
	}
}

// TestImportDoesNotMutateInput verifies Import leaves the caller's slice
// untouched, matching Filter's contract.
func TestImportDoesNotMutateInput(t *testing.T) {
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	existing, _ := mustImport(t, nil, `[{"global_id":"a:1","title":"Old","company":"acme"}]`, "acme", now)
	snapshot := existing[0]
	if _, _, err := Import(existing, []byte(`[{"global_id":"a:1","title":"New","company":"acme"}]`), "acme", now); err != nil {
		t.Fatal(err)
	}
	if existing[0] != snapshot {
		t.Errorf("Import mutated its input: got %+v, want %+v", existing[0], snapshot)
	}
}

// TestImportRejectsEmptyEmployer verifies a role with no resolvable employer
// (no payload company and no --company label) is dropped, not stored blank.
func TestImportRejectsEmptyEmployer(t *testing.T) {
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	out, res, err := Import(nil, []byte(`[{"global_id":"x:1","title":"Ghost"}]`), "", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 || len(res.New) != 0 {
		t.Errorf("empty-employer role should be rejected, got %d roles / %d new", len(out), len(res.New))
	}
}

// TestImportSkipGoneMarking verifies SkipGoneMarking leaves roles absent from a
// partial payload open instead of retiring them.
func TestImportSkipGoneMarking(t *testing.T) {
	now1 := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	first := `[
	  {"global_id":"gh:1","title":"SRE","company":"Acme","url":"https://x/1"},
	  {"global_id":"gh:2","title":"PM","company":"Acme","url":"https://x/2"}
	]`
	roles1, _ := mustImport(t, nil, first, "acme", now1)

	// A partial second payload drops gh:2. With SkipGoneMarking it stays open.
	second := `[{"global_id":"gh:1","title":"SRE","company":"Acme","url":"https://x/1"}]`
	now2 := time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)
	roles2, res, err := Import(roles1, []byte(second), "acme", now2, SkipGoneMarking())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, g := res.Counts(); g != 0 {
		t.Errorf("gone = %d, want 0 with SkipGoneMarking", g)
	}
	byID := map[string]model.Role{}
	for _, r := range roles2 {
		byID[r.GlobalID] = r
	}
	if byID["gh:2"].Status != model.RoleOpen {
		t.Errorf("gh:2 status = %q, want open (not retired)", byID["gh:2"].Status)
	}
}

func TestOpenCountForCompany(t *testing.T) {
	idx := []model.Role{
		{GlobalID: "a:1", Company: "acme", Status: model.RoleOpen},
		{GlobalID: "a:2", Company: "acme", Status: model.RoleGone},
		{GlobalID: "a:3", Employer: "Acme", Status: model.RoleOpen}, // legacy: slug from employer
		{GlobalID: "b:1", Company: "beta", Status: model.RoleOpen},
	}
	if got := OpenCountForCompany(idx, "acme"); got != 2 {
		t.Errorf("OpenCountForCompany(acme) = %d, want 2", got)
	}
	if got := OpenCountForCompany(idx, ""); got != 0 {
		t.Errorf("OpenCountForCompany(empty) = %d, want 0", got)
	}
}

func sampleIndex() []model.Role {
	return []model.Role{
		{GlobalID: "a:1", Title: "Senior Engineer", Employer: "Acme Corp", Description: "golang",
			Remote: true, FirstSeen: "2026-06-15", LastSeen: "2026-06-18", Status: model.RoleOpen},
		{GlobalID: "a:2", Title: "Designer", Employer: "Acme Corp", Description: "figma",
			Remote: false, FirstSeen: "2026-06-01", LastSeen: "2026-06-01", Status: model.RoleOpen},
		{GlobalID: "b:1", Title: "Data Scientist", Employer: "Beta LLC", Description: "python",
			Remote: true, FirstSeen: "2026-05-01", LastSeen: "2026-05-10", Status: model.RoleGone},
	}
}

func ids(roles []model.Role) []string {
	out := make([]string, len(roles))
	for i, r := range roles {
		out[i] = r.GlobalID
	}
	return out
}

func eqIDs(got []model.Role, want ...string) bool {
	g := ids(got)
	if len(g) != len(want) {
		return false
	}
	for i := range g {
		if g[i] != want[i] {
			return false
		}
	}
	return true
}

func TestFilter(t *testing.T) {
	since := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	remoteTrue := true

	tests := []struct {
		name string
		q    Query
		want []string
	}{
		{"default open only", Query{}, []string{"a:1", "a:2"}},
		{"gone", Query{Gone: true}, []string{"b:1"}},
		{"new since", Query{Since: since, New: true}, []string{"a:1"}},
		{"changed since", Query{Since: since, Changed: true}, []string{"a:1"}},
		{"employer substring", Query{Employer: "acme"}, []string{"a:1", "a:2"}},
		{"title substring", Query{Title: "engineer"}, []string{"a:1"}},
		{"search description", Query{Search: "figma"}, []string{"a:2"}},
		{"remote true", Query{Remote: &remoteTrue}, []string{"a:1"}},
		{"since only", Query{Since: since}, []string{"a:1"}},
		{"new no since is noop", Query{New: true}, []string{"a:1", "a:2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Filter(sampleIndex(), tt.q)
			if !eqIDs(got, tt.want...) {
				t.Errorf("Filter() = %v, want %v", ids(got), tt.want)
			}
		})
	}
}

func TestFilterLane(t *testing.T) {
	index := []model.Role{
		{GlobalID: "l:1", Title: "Staff Site Reliability Engineer", Employer: "a", Status: model.RoleOpen},
		{GlobalID: "l:2", Title: "Principal Developer Advocate", Employer: "a", Status: model.RoleOpen},
		{GlobalID: "l:3", Title: "Solutions Architect", Employer: "a", Status: model.RoleOpen},
		{GlobalID: "l:4", Title: "Senior SRE", Employer: "a", Status: model.RoleOpen},
		{GlobalID: "l:5", Title: "Field CTO", Employer: "a", Status: model.RoleOpen},
		{GlobalID: "l:6", Title: "Staff Solutions Engineer", Employer: "a", Status: model.RoleOpen},
		{GlobalID: "l:7", Title: "Accounts Payable Specialist", Employer: "a", Status: model.RoleOpen},
	}

	tests := []struct {
		lane string
		want []string
	}{
		{"reliability", []string{"l:1", "l:4"}},
		{"devex", []string{"l:2", "l:5"}},
		{"fixer", []string{"l:3", "l:6"}},
	}
	for _, tt := range tests {
		t.Run(tt.lane, func(t *testing.T) {
			got := Filter(index, Query{Lane: tt.lane})
			if !eqIDs(got, tt.want...) {
				t.Errorf("Filter(lane=%q) = %v, want %v", tt.lane, ids(got), tt.want)
			}
		})
	}
}

func TestFilterLaneCustomConfig(t *testing.T) {
	custom := map[string][]string{
		"eng": {"software engineer", "backend engineer"},
	}
	index := []model.Role{
		{GlobalID: "c:1", Title: "Senior Software Engineer", Employer: "x", Status: model.RoleOpen},
		{GlobalID: "c:2", Title: "Backend Engineer", Employer: "x", Status: model.RoleOpen},
		{GlobalID: "c:3", Title: "Product Designer", Employer: "x", Status: model.RoleOpen},
	}
	got := Filter(index, Query{Lane: "eng", Lanes: custom})
	if !eqIDs(got, "c:1", "c:2") {
		t.Errorf("custom lane filter = %v, want [c:1 c:2]", ids(got))
	}
}

func TestFilterLaneUnknown(t *testing.T) {
	index := []model.Role{
		{GlobalID: "u:1", Title: "Software Engineer", Employer: "x", Status: model.RoleOpen},
	}
	got := Filter(index, Query{Lane: "nonexistent"})
	if len(got) != 0 {
		t.Errorf("unknown lane should match nothing, got %v", ids(got))
	}
}

func TestFind(t *testing.T) {
	roles := []model.Role{
		{GlobalID: "ashby:abc123"},
		{GlobalID: "ashby:abd999"},
		{GlobalID: "lever:zzz"},
	}

	if r, ok := Find(roles, "ashby:abc123"); !ok || r.GlobalID != "ashby:abc123" {
		t.Errorf("full id: got (%q,%v)", r.GlobalID, ok)
	}
	if r, ok := Find(roles, "ashby:abc"); !ok || r.GlobalID != "ashby:abc123" {
		t.Errorf("unambiguous prefix: got (%q,%v)", r.GlobalID, ok)
	}
	if _, ok := Find(roles, "ashby:ab"); ok {
		t.Errorf("ambiguous prefix: want ok=false")
	}
	if _, ok := Find(roles, "nope"); ok {
		t.Errorf("no match: want ok=false")
	}
	if _, ok := Find(roles, ""); ok {
		t.Errorf("empty: want ok=false")
	}
}

// TestImportSynthesizesKeyFromATS verifies that when jobhive omits global_id
// (the live scrape feed does), the index key is the canonical ats_type:ats_id,
// not the long url.
func TestImportSynthesizesKeyFromATS(t *testing.T) {
	payload := `[{"title":"Senior AI Software Engineer","company":"Solo.io","ats_type":"greenhouse","ats_id":"4661637005","url":"https://job-boards.greenhouse.io/soloioinc/jobs/4661637005"}]`
	updated, res, err := Import(nil, []byte(payload), "solo-io", time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 {
		t.Fatalf("want 1 role, got %d", len(updated))
	}
	if got := updated[0].GlobalID; got != "greenhouse:4661637005" {
		t.Errorf("GlobalID = %q, want greenhouse:4661637005", got)
	}
	if len(res.New) != 1 || res.New[0] != "greenhouse:4661637005" {
		t.Errorf("res.New = %v", res.New)
	}
}
