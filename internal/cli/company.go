package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bttnns/joblog/internal/catalog"
	"github.com/bttnns/joblog/internal/model"
	"github.com/bttnns/joblog/internal/store"
	"github.com/spf13/cobra"
)

func init() { addCommand(newCompanyCmd) }

// newCompanyCmd groups the company verbs under `jl company`. A bare `jl company`
// runs ls. The company list is your scrape rotation plus a per-company research
// folder; status (active or paused) is one you set.
func newCompanyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "company",
		Aliases: []string{"target"},
		Short:   "Companies you track: the scrape rotation plus per-company research",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompanyList(cmd, false)
		},
	}
	cmd.AddCommand(
		newCompanyAddCmd(),
		newCompanySearchCmd(),
		newCompanyListCmd(),
		newCompanySetCmd(),
		newCompanyRmCmd(),
		newCompanyShowCmd(),
	)
	return cmd
}

func newCompanyAddCmd() *cobra.Command {
	var c store.Company
	cmd := &cobra.Command{
		Use:   "add [url-or-slug...]",
		Short: "Track one or more companies and scaffold each research folder",
		Long: "Add companies to track. Each argument is either a careers or posting URL (a\n" +
			"board root or a specific posting link), or a catalog slug from `jl company\n" +
			"search`. jl reads the ATS and slug from a URL, e.g.\n" +
			"  jl company add https://boards.greenhouse.io/acme https://jobs.lever.co/globex\n" +
			"or resolves a bare slug against the built-in snapshot, e.g.\n" +
			"  jl company add acme globex          (add --ats greenhouse if a slug is ambiguous)\n" +
			"Read URLs or slugs from stdin with '-'. New companies start active.\n\n" +
			"For a board jl does not recognize and that is not in the snapshot (a single-tenant\n" +
			"or custom site), pass the flags by hand instead: --name --ats --slug --careers-url.\n" +
			"Those companies stay custom; scrape them yourself and pipe the result to jl role\n" +
			"import.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()

			toAdd, err := companiesToAdd(c, args)
			if err != nil {
				return err
			}

			companies, err := s.LoadCompanies()
			if err != nil {
				return err
			}
			var saved []store.Company
			for _, nc := range toAdd {
				replaced := false
				for i := range companies {
					if strings.EqualFold(companies[i].Name, nc.Name) {
						// Preserve an existing status on update unless re-adding flips it.
						if nc.Status == "" {
							nc.Status = companies[i].Status
						}
						companies[i] = nc
						replaced = true
						break
					}
				}
				if !replaced {
					companies = append(companies, nc)
				}
				if err := scaffoldCompanyFolder(s, nc); err != nil {
					return err
				}
				saved = append(saved, nc)
				if !wantJSON(cmd) {
					verb := "Added"
					if replaced {
						verb = "Updated"
					}
					info("%s company %s (%s/%s)", verb, nc.Name, nc.ATS, nc.Slug)
					if nc.ATS != "" && nc.ATS != "custom" && nc.Slug != "" {
						info("  fetch its roles with: jl fetch %s", nc.Name)
					}
				}
			}
			if err := s.SaveCompanies(companies); err != nil {
				return err
			}
			if wantJSON(cmd) {
				return emitJSON(saved)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&c.Name, "name", "", "company name (the key, and the slug that links roles and applications)")
	f.StringVar(&c.ATS, "ats", "", "applicant tracking system (greenhouse, ashby, lever, ...)")
	f.StringVar(&c.Slug, "slug", "", "the slug the scraper needs")
	f.StringVar(&c.CareersURL, "careers-url", "", "public careers URL")
	return cmd
}

// companiesToAdd resolves the companies to add from positional URL args (each
// parsed into ATS + slug, with '-' reading newline-separated URLs from stdin),
// or, with no URL args, from the explicit flags in base for the custom case.
func companiesToAdd(base store.Company, args []string) ([]store.Company, error) {
	var urls []string
	for _, a := range args {
		if a == "-" {
			lines, err := readURLsFromStdin()
			if err != nil {
				return nil, err
			}
			urls = append(urls, lines...)
			continue
		}
		urls = append(urls, a)
	}

	if len(urls) == 0 {
		// No URLs: the explicit-flag (custom/fallback) path.
		if base.Name == "" {
			return nil, fmt.Errorf("pass a careers URL or a catalog slug, or use --name with --ats/--slug for a custom board")
		}
		if base.Status == "" {
			base.Status = store.StatusActive
		}
		return []store.Company{base}, nil
	}

	var out []store.Company
	for _, raw := range urls {
		c, err := resolveCompanyArg(base, raw)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// resolveCompanyArg turns one positional `company add` argument into a Company.
// A URL is parsed for its ATS and slug as before. An argument that is not a URL
// is resolved against the embedded catalog as a slug (scoped to --ats when given,
// which also disambiguates a slug shared across ATSes), so a `jl company search`
// hit can be added by its slug. The custom/flag fallbacks are preserved.
func resolveCompanyArg(base store.Company, raw string) (store.Company, error) {
	// A bare token (no scheme, no dot, no slash) is treated as a catalog slug
	// rather than a URL, so a `jl company search` hit can be added by its slug. A
	// URL-shaped argument with a known host but no slug should report the URL
	// error, not a catalog miss, so only the bare-token case falls through here.
	if looksLikeSlug(raw) {
		cat, err := resolveCatalogSlug(raw, base.ATS)
		if err != nil {
			return store.Company{}, err
		}
		return store.Company{
			Name:       firstNonEmpty(base.Name, cat.Name),
			ATS:        cat.ATS,
			Slug:       cat.Slug,
			CareersURL: catalog.URL(cat.ATS, cat.Slug),
			Status:     store.StatusActive,
		}, nil
	}
	m, err := parseATSURL(raw)
	if err != nil {
		return store.Company{}, err
	}
	c := store.Company{
		ATS:        m.ATS,
		Slug:       m.Slug,
		CareersURL: raw,
		Status:     store.StatusActive,
	}
	if m.ATS == "" {
		// Unrecognized host: stays custom, leaning on any flags the user passed.
		c.ATS = firstNonEmpty(base.ATS, "custom")
		c.Slug = base.Slug
		c.Name = base.Name
	}
	if c.Name == "" {
		c.Name = firstNonEmpty(base.Name, companyNameFromSlug(c.Slug))
	}
	if c.Name == "" {
		return store.Company{}, fmt.Errorf("could not derive a name from %q; pass --name", raw)
	}
	return c, nil
}

// looksLikeSlug reports whether raw is a bare catalog slug rather than a URL: no
// scheme, no path separator, and no dot (a dot implies a hostname). parseATSURL
// already rejects a dotless token as "not a URL"; this gate just routes such a
// token to the catalog before the URL parser runs, so a real URL with a known
// host but a missing slug still gets the URL parser's clearer error.
func looksLikeSlug(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	return !strings.Contains(raw, "://") && !strings.Contains(raw, "/") && !strings.Contains(raw, ".")
}

// readURLsFromStdin reads newline-separated URLs from stdin, skipping blanks. It
// refuses an interactive terminal so `jl company add -` does not hang waiting on
// a tty with nothing piped in.
func readURLsFromStdin() ([]string, error) {
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		return nil, fmt.Errorf("no input: pipe URLs in (e.g. printf '%%s\\n' <url> | jl company add -)")
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

func newCompanyListCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List tracked companies (active only by default; --all includes paused)",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompanyList(cmd, all)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include paused companies (default shows only active)")
	return cmd
}

// companyRow is one row of `jl company ls`, carrying the status field plus the
// derived data columns (open roles and applications).
type companyRow struct {
	store.Company
	Roles   int `json:"roles"`
	Applied int `json:"applied"`
}

func runCompanyList(cmd *cobra.Command, all bool) error {
	s, err := openStore(cmd)
	if err != nil {
		return err
	}
	companies, err := s.LoadCompanies()
	if err != nil {
		return err
	}
	log, err := s.LoadLog()
	if err != nil {
		return err
	}
	rolesIdx, err := s.LoadRoles()
	if err != nil {
		return err
	}
	applied := appliedCounts(log)
	roleCounts := openRoleCounts(rolesIdx)

	var rows []companyRow
	for _, c := range companies {
		if !all && c.Status != store.StatusActive {
			continue
		}
		slug := model.Slug(c.Name)
		rows = append(rows, companyRow{c, roleCounts[slug], applied[slug]})
	}
	if wantJSON(cmd) {
		return emitJSON(rows)
	}
	if len(rows) == 0 {
		info("no companies; add one with: jl company add <careers-url>")
		return nil
	}
	tw := newTabWriter()
	fmt.Fprintln(tw, "NAME\tSTATUS\tATS\tSLUG\tROLES\tAPPLIED\tCAREERS-URL")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\t%s\n", r.Name, r.Status, r.ATS, r.Slug, r.Roles, r.Applied, r.CareersURL)
	}
	return tw.Flush()
}

func newCompanySetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <name> active|paused",
		Short: "Set a company's status: active (in the fetch rotation) or paused",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, status := args[0], strings.ToLower(args[1])
			if status != store.StatusActive && status != store.StatusPaused {
				return fmt.Errorf("invalid status %q: want %s or %s", args[1], store.StatusActive, store.StatusPaused)
			}
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			companies, err := s.LoadCompanies()
			if err != nil {
				return err
			}
			var found *store.Company
			for i := range companies {
				if strings.EqualFold(companies[i].Name, name) {
					companies[i].Status = status
					found = &companies[i]
					break
				}
			}
			if found == nil {
				return fmt.Errorf("no company named %q", name)
			}
			if err := s.SaveCompanies(companies); err != nil {
				return err
			}
			if wantJSON(cmd) {
				return emitJSON(found)
			}
			info("Set %s to %s", found.Name, status)
			return nil
		},
	}
}

func newCompanyRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "Stop tracking a company (its research folder is kept)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			companies, err := s.LoadCompanies()
			if err != nil {
				return err
			}
			out := companies[:0]
			removed := false
			for _, c := range companies {
				if strings.EqualFold(c.Name, args[0]) {
					removed = true
					continue
				}
				out = append(out, c)
			}
			if !removed {
				return fmt.Errorf("no company named %q", args[0])
			}
			if err := s.SaveCompanies(out); err != nil {
				return err
			}
			info("Stopped tracking %s", args[0])
			return nil
		},
	}
}

func newCompanyShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "show <name>",
		Aliases: []string{"get"},
		Short:   "Show a company's status, open roles, applications, and research files",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore(cmd)
			if err != nil {
				return err
			}
			companies, err := s.LoadCompanies()
			if err != nil {
				return err
			}
			var found *store.Company
			for i := range companies {
				if strings.EqualFold(companies[i].Name, args[0]) {
					found = &companies[i]
					break
				}
			}
			if found == nil {
				return fmt.Errorf("no company named %q", args[0])
			}
			slug := model.Slug(found.Name)
			log, _ := s.LoadLog()
			rolesIdx, _ := s.LoadRoles()
			roleN := openRoleCounts(rolesIdx)[slug]
			appliedN := appliedCounts(log)[slug]
			researchFiles := companyFiles(s, found.Name)
			if wantJSON(cmd) {
				return emitJSON(map[string]any{
					"company":        found,
					"open_roles":     roleN,
					"applied":        appliedN,
					"research_files": researchFiles,
				})
			}
			fmt.Println(found.Name)
			fmt.Printf("  Status: %s\n", found.Status)
			fmt.Printf("  ATS: %s   Slug: %s\n", found.ATS, found.Slug)
			fmt.Printf("  Careers: %s\n", found.CareersURL)
			fmt.Printf("  Open roles: %d   Applications: %d\n", roleN, appliedN)
			fmt.Printf("  Folder: %s\n", companyDir(s, found.Name))
			if len(researchFiles) == 0 {
				fmt.Println("  Research: (none yet)")
			} else {
				fmt.Println("  Research:")
				for _, f := range researchFiles {
					fmt.Printf("    %s\n", f)
				}
			}
			return nil
		},
	}
}

// companyDir is the per-company research folder, keyed by the canonical slug.
func companyDir(s *store.Store, name string) string {
	return s.Path("companies", model.Slug(name))
}

// scaffoldCompanyFolder creates companies/<slug>/company.md if absent. It never
// clobbers existing research, and writes atomically via the store like every
// other artifact.
func scaffoldCompanyFolder(s *store.Store, c store.Company) error {
	rel := filepath.Join("companies", model.Slug(c.Name), "company.md")
	if _, err := os.Stat(s.Path(rel)); err == nil {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", c.Name)
	fmt.Fprintf(&b, "- ATS: %s\n- Slug: %s\n- Careers: %s\n\n", c.ATS, c.Slug, c.CareersURL)
	b.WriteString("## Why interested\n\nTODO\n\n## Research\n\nTODO\n")
	return s.WriteFile(rel, []byte(b.String()))
}

// companyFiles lists the files in a company's research folder (sorted).
func companyFiles(s *store.Store, name string) []string {
	entries, err := os.ReadDir(companyDir(s, name))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// appliedCounts counts log entries per company slug, so the company list can
// show how many applications and activities are linked to each. Legacy entries
// with no Company fall back to the slug of their employer.
func appliedCounts(log []model.Entry) map[string]int {
	out := map[string]int{}
	for _, e := range log {
		slug := e.Company
		if slug == "" {
			slug = model.Slug(e.Employer)
		}
		if slug != "" {
			out[slug]++
		}
	}
	return out
}

// openRoleCounts counts open roles per company slug, with the same legacy
// employer fallback as appliedCounts.
func openRoleCounts(rs []model.Role) map[string]int {
	out := map[string]int{}
	for _, r := range rs {
		if r.Status != model.RoleOpen {
			continue
		}
		slug := r.Company
		if slug == "" {
			slug = model.Slug(r.Employer)
		}
		if slug != "" {
			out[slug]++
		}
	}
	return out
}
