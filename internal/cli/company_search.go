package cli

import (
	"fmt"
	"strings"

	"github.com/bttnns/joblog/internal/catalog"
	"github.com/bttnns/joblog/internal/model"
	"github.com/bttnns/joblog/internal/store"
	"github.com/spf13/cobra"
)

// searchResult is one row of `jl company search`: a catalog company, its derived
// careers URL, and whether you already track it (and at what status).
type searchResult struct {
	catalog.Company
	URL     string `json:"url"`
	Tracked string `json:"tracked"` // active | paused | untracked
}

func newCompanySearchCmd() *cobra.Command {
	var (
		ats   string
		limit int
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Fuzzy-search the built-in company snapshot (no network)",
		Long: "Search a built-in snapshot of public ATS companies (about 40k, embedded in jl;\n" +
			"refreshed with `make catalog`). This makes no network call. Results are ranked\n" +
			"by name match and show NAME, ATS, SLUG, the derived careers URL, and whether you\n" +
			"already track that company (active, paused, or untracked).\n\n" +
			"Add one you like by slug or URL:\n" +
			"  jl company add <slug>            (scope with --ats if the slug is ambiguous)\n" +
			"  jl company add <careers-url>\n\n" +
			"The snapshot is a convenience for discovery; a company not in it can still be\n" +
			"tracked by passing its careers URL or the --ats/--slug flags to `jl company add`.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			hits, err := catalog.Search(query, ats, limit)
			if err != nil {
				return err
			}

			s, err := openStore(cmd)
			if err != nil {
				return err
			}
			tracked, err := trackedIndex(s)
			if err != nil {
				return err
			}

			rows := make([]searchResult, 0, len(hits))
			for _, h := range hits {
				rows = append(rows, searchResult{
					Company: h.Company,
					URL:     h.URL(),
					Tracked: trackedStatus(tracked, h.Company),
				})
			}

			if wantJSON(cmd) {
				return emitJSON(rows)
			}
			if len(rows) == 0 {
				info("no matches in the catalog for %q; try fewer characters or a different spelling", query)
				return nil
			}
			tw := newTabWriter()
			fmt.Fprintln(tw, "NAME\tATS\tSLUG\tTRACKED\tURL")
			for _, r := range rows {
				url := r.URL
				if url == "" {
					url = "(careers url unknown)"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Name, r.ATS, r.Slug, r.Tracked, url)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			info("add one with: jl company add <slug>   (or its careers URL)")
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&ats, "ats", "", "restrict to one ATS (greenhouse, ashby, lever, ...)")
	f.IntVar(&limit, "limit", 20, "maximum results to show")
	return cmd
}

// trackedIndex builds a lookup of the companies you already track, keyed by both
// "ats/slug" and bare slug, so a catalog hit can be marked tracked. The value is
// the tracked company's status (active or paused).
func trackedIndex(s *store.Store) (map[string]string, error) {
	companies, err := s.LoadCompanies()
	if err != nil {
		return nil, err
	}
	idx := make(map[string]string, len(companies)*2)
	for _, c := range companies {
		status := c.Status
		if status == "" {
			status = store.StatusActive
		}
		if c.Slug != "" {
			idx[strings.ToLower(c.ATS+"/"+c.Slug)] = status
			// Bare slug too, for a tracked company added without an ATS.
			if _, ok := idx[strings.ToLower(c.Slug)]; !ok {
				idx[strings.ToLower(c.Slug)] = status
			}
		}
		// Also match on the canonical name slug, so a company tracked by name lines
		// up with a catalog row whose slug equals that canonical slug.
		if ns := model.Slug(c.Name); ns != "" {
			if _, ok := idx[ns]; !ok {
				idx[ns] = status
			}
		}
	}
	return idx, nil
}

// trackedStatus reports whether a catalog company is tracked, preferring an
// ats/slug match before falling back to a bare-slug match. It returns "active",
// "paused", or "untracked".
func trackedStatus(idx map[string]string, c catalog.Company) string {
	if st, ok := idx[strings.ToLower(c.ATS+"/"+c.Slug)]; ok {
		return st
	}
	if st, ok := idx[strings.ToLower(c.Slug)]; ok {
		return st
	}
	return "untracked"
}

// resolveCatalogSlug resolves a bare slug (not a URL) against the embedded
// catalog. With ats set, it matches only that ATS. It returns the single matching
// company; zero matches or, when ats is unset, matches across multiple ATSes are
// errors that tell the caller how to disambiguate.
func resolveCatalogSlug(slug, ats string) (catalog.Company, error) {
	all, err := catalog.All()
	if err != nil {
		return catalog.Company{}, err
	}
	wantSlug := strings.ToLower(strings.TrimSpace(slug))
	wantATS := strings.ToLower(strings.TrimSpace(ats))
	var matches []catalog.Company
	for _, c := range all {
		if strings.ToLower(c.Slug) != wantSlug {
			continue
		}
		if wantATS != "" && strings.ToLower(c.ATS) != wantATS {
			continue
		}
		matches = append(matches, c)
	}
	switch len(matches) {
	case 0:
		if wantATS != "" {
			return catalog.Company{}, fmt.Errorf("no catalog company with slug %q on ATS %q; pass a careers URL or --name/--ats/--slug", slug, ats)
		}
		return catalog.Company{}, fmt.Errorf("no catalog company with slug %q; pass a careers URL or --name/--ats/--slug", slug)
	case 1:
		return matches[0], nil
	default:
		var atses []string
		for _, m := range matches {
			atses = append(atses, m.ATS)
		}
		return catalog.Company{}, fmt.Errorf("slug %q is ambiguous across ATSes (%s); narrow it with --ats", slug, strings.Join(atses, ", "))
	}
}
