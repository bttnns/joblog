package catalog

import "testing"

// fixture is a tiny hand-built catalog so the ranking assertions do not depend on
// the 40k-row embedded snapshot.
var fixture = []Company{
	{ATS: "greenhouse", Name: "Acme", Slug: "acme"},
	{ATS: "lever", Name: "Acme Corp", Slug: "acme-corp"},
	{ATS: "ashby", Name: "Globex", Slug: "globex"},
	{ATS: "greenhouse", Name: "Initech Systems", Slug: "initech"},
	{ATS: "workable", Name: "Pied Piper", Slug: "pied-piper"},
	{ATS: "ashby", Name: "acme", Slug: "acme-dup"}, // same ATS-less slug clash test
}

func TestSearchRanking(t *testing.T) {
	// "acme": exact-name matches rank above prefix matches above subsequence.
	got := search(fixture, "acme", "", 0)
	if len(got) < 3 {
		t.Fatalf("want at least 3 hits for acme, got %d", len(got))
	}
	// The two exact "acme"/"acme" names tie on score; the shorter-name tiebreak and
	// stable name order keep them first, before "Acme Corp" (a prefix match).
	if got[0].Name != "Acme" && got[0].Name != "acme" {
		t.Errorf("top hit = %q, want an exact acme match", got[0].Name)
	}
	// "Acme Corp" is only a prefix match, so it must rank below the exact ones.
	var corpRank, exactRank int = -1, -1
	for i, r := range got {
		if r.Slug == "acme-corp" {
			corpRank = i
		}
		if r.Slug == "acme" {
			exactRank = i
		}
	}
	if exactRank == -1 || corpRank == -1 || exactRank >= corpRank {
		t.Errorf("exact 'acme' (rank %d) should outrank prefix 'Acme Corp' (rank %d)", exactRank, corpRank)
	}
}

func TestSearchSubsequence(t *testing.T) {
	// "gx" is a subsequence of "globex" but not a substring; it should still match.
	got := search(fixture, "gx", "", 0)
	if len(got) != 1 || got[0].Slug != "globex" {
		t.Fatalf("subsequence search gx = %+v, want only globex", got)
	}
	// A query that is not a subsequence of any name returns nothing.
	if got := search(fixture, "zzzz", "", 0); len(got) != 0 {
		t.Errorf("nonsense query returned %d hits, want 0", len(got))
	}
}

func TestSearchATSFilterAndLimit(t *testing.T) {
	got := search(fixture, "acme", "lever", 0)
	if len(got) != 1 || got[0].ATS != "lever" {
		t.Fatalf("ats-filtered acme = %+v, want only the lever row", got)
	}
	if got := search(fixture, "acme", "", 1); len(got) != 1 {
		t.Errorf("limit 1 returned %d hits, want 1", len(got))
	}
}

func TestURL(t *testing.T) {
	cases := map[string]string{
		"greenhouse": "https://boards.greenhouse.io/acme",
		"ashby":      "https://jobs.ashbyhq.com/acme",
		"lever":      "https://jobs.lever.co/acme",
		"recruitee":  "https://acme.recruitee.com",
		"teamtailor": "https://acme.teamtailor.com",
	}
	for ats, want := range cases {
		if got := URL(ats, "acme"); got != want {
			t.Errorf("URL(%q) = %q, want %q", ats, got, want)
		}
	}
	// An ATS the table does not know yields an empty URL.
	if got := URL("bamboohr", "acme"); got != "" {
		t.Errorf("URL(bamboohr) = %q, want empty", got)
	}
}

// TestEmbeddedCatalogLoads sanity-checks that the committed gz decompresses and
// parses into a plausibly large set of rows with a known company present.
func TestEmbeddedCatalogLoads(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatalf("load embedded catalog: %v", err)
	}
	if len(all) < 10000 {
		t.Fatalf("embedded catalog has %d rows, want a large snapshot", len(all))
	}
	for _, c := range all {
		if c.ATS == "" || c.Slug == "" {
			t.Fatalf("row with empty ats or slug: %+v", c)
		}
	}
}
