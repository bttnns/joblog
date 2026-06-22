package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/bttnns/joblog/internal/dates"
	"github.com/bttnns/joblog/internal/model"
	"github.com/bttnns/joblog/internal/roles"
	"github.com/bttnns/joblog/internal/store"
	"github.com/spf13/cobra"
)

func init() { addCommand(newRolesCmd) }

func newRolesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "role",
		Aliases: []string{"roles"},
		Short:   "The deduped index of roles seen over time (new, changed, gone)",
		Long: "jl keeps a local index of every role it has seen and surfaces what is new,\n" +
			"changed, or gone. Scraping happens outside jl (via jobhive, JobSpy, or a curl\n" +
			"export); role import ingests the resulting JSON. A bare 'jl role' lists them.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRolesList(cmd)
		},
	}
	cmd.AddCommand(
		newRolesImportCmd(),
		newRolesListCmd(),
		newRolesChangesCmd(),
		newRolesGetCmd(),
		newRolesRmCmd(),
	)
	return cmd
}

func newRolesImportCmd() *cobra.Command {
	var company string
	var noGone, force bool
	cmd := &cobra.Command{
		Use:   "import [file|-]",
		Short: "Ingest role JSON from jobhive or a curl export",
		Long: "Read an ATS job JSON payload from a file or stdin, upsert it into the index,\n" +
			"mark roles gone for that company when they vanish, and print the delta.\n\n" +
			"  jobhive scrape ashby acme --format json | jl role import - --company acme\n\n" +
			"A full scrape of a company is expected. For a deliberately partial import (one\n" +
			"page, a filtered export) pass --no-gone so missing roles are not retired.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(company) == "" {
				return fmt.Errorf("--company is required (it scopes gone-marking and links roles to the company)")
			}
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			payload, err := readInput(args)
			if err != nil {
				return err
			}
			res, updated, err := importPayload(s, payload, company, noGone, force, nowFunc())
			if err != nil {
				return err
			}

			if wantJSON(cmd) {
				return emitJSON(res)
			}
			newN, changedN, goneN := res.Counts()
			info("Imported %s: %d new, %d changed, %d gone", company, newN, changedN, goneN)
			for _, id := range res.New {
				if r, ok := roles.Find(updated, id); ok {
					info("  new:     %s  %s", r.Employer, r.Title)
				}
			}
			for _, id := range res.Changed {
				if r, ok := roles.Find(updated, id); ok {
					info("  changed: %s  %s", r.Employer, r.Title)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&company, "company", "", "company label for this import (required; scopes gone-marking)")
	cmd.Flags().BoolVar(&noGone, "no-gone", false, "do not retire roles absent from this payload (for a partial import)")
	cmd.Flags().BoolVar(&force, "force", false, "apply even when the import would retire most of a company's open roles")
	return cmd
}

// importPayload is the shared import core behind both `role import` and `fetch`.
// It loads the current index, imports payload under company (the canonical slug
// scopes gone-marking), applies the destructive-import guard, then saves the
// index, records the last-import delta, and archives the raw payload. It returns
// the delta plus the updated index so callers can print per-role detail. now
// stamps first_seen/last_seen and the archive path.
func importPayload(s *store.Store, payload []byte, company string, noGone, force bool, now time.Time) (roles.ImportResult, []model.Role, error) {
	existing, err := s.LoadRoles()
	if err != nil {
		return roles.ImportResult{}, nil, err
	}

	var opts []roles.ImportOption
	if noGone {
		opts = append(opts, roles.SkipGoneMarking())
	}
	updated, res, err := roles.Import(existing, payload, company, now, opts...)
	if err != nil {
		return roles.ImportResult{}, nil, err
	}

	// Destructive-import guard: a truncated or partial scrape (e.g. the known
	// large-stdin truncation bug in some agent harnesses) would mark most of a
	// company's open roles gone. Refuse when an import would retire more than half
	// of a non-trivial set, unless --force or --no-gone says it is intended.
	if !noGone && !force {
		_, _, goneN := res.Counts()
		prior := roles.OpenCountForCompany(existing, model.Slug(company))
		if prior >= pruneGuardFloor && goneN*2 > prior {
			return roles.ImportResult{}, nil, fmt.Errorf("this import would mark %d of %d open roles at %q gone (>50%%), which usually means a truncated or partial scrape; re-run with --force if intended, or --no-gone to import without retiring any", goneN, prior, company)
		}
	}

	if err := s.SaveRoles(updated); err != nil {
		return roles.ImportResult{}, nil, err
	}
	if err := s.WriteJSON(roles.LastImportRelPath, res); err != nil {
		return roles.ImportResult{}, nil, err
	}
	// Archive the raw payload for provenance (best-effort).
	_ = s.WriteFile(roles.ImportArchiveRelPath(now.Format(dates.ISO), company), payload)

	return res, updated, nil
}

// pruneGuardFloor is the smallest company open-role count at which the
// destructive-import guard engages. Below it, retiring "most" roles is too
// small a number to be a meaningful signal of a truncated scrape.
const pruneGuardFloor = 4

func newRolesListCmd() *cobra.Command {
	var since, employer, companyDeprecated, title, search, lane string
	var onlyNew, onlyChanged, onlyGone, remote bool
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "Query the roles index (pre-filtered, no description)",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRolesList(cmd)
		},
	}
	f := cmd.Flags()
	f.StringVar(&since, "since", "", "only roles active on/after a date or window (e.g. 7d, 2w, 2026-03-16)")
	f.BoolVar(&onlyNew, "new", false, "only roles first seen within --since")
	f.BoolVar(&onlyChanged, "changed", false, "only roles changed within --since")
	f.BoolVar(&onlyGone, "gone", false, "only roles now gone")
	f.StringVar(&employer, "employer", "", "filter by employer (substring)")
	f.StringVar(&companyDeprecated, "company", "", "deprecated alias for --employer")
	_ = f.MarkDeprecated("company", "use --employer (it matches the display employer, not the company slug)")
	f.StringVar(&title, "title", "", "filter by title (substring)")
	f.StringVar(&search, "search", "", "substring match on title, employer, description")
	f.BoolVar(&remote, "remote", false, "filter by remote; pass --remote=false to require on-site, omit for either")
	f.StringVar(&lane, "lane", "", "filter by role type (reliability, devex, fixer); edit lanes.yaml to customize")
	return cmd
}

// runRolesList renders the roles index, reading any filter flags from cmd. It is
// shared by `jl role ls` and the bare `jl role` group (which carries no filter
// flags and so lists everything). flagStr/flagBool tolerate a missing flag.
func runRolesList(cmd *cobra.Command) error {
	s, err := openStore(cmd)
	if err != nil {
		return err
	}
	all, err := s.LoadRoles()
	if err != nil {
		return err
	}
	q := roles.Query{
		New:      flagBool(cmd, "new"),
		Changed:  flagBool(cmd, "changed"),
		Gone:     flagBool(cmd, "gone"),
		Employer: firstNonEmpty(flagStr(cmd, "employer"), flagStr(cmd, "company")),
		Title:    flagStr(cmd, "title"),
		Search:   flagStr(cmd, "search"),
	}
	if since := flagStr(cmd, "since"); since != "" {
		cut, err := dates.ParseSince(nowFunc(), since)
		if err != nil {
			return err
		}
		q.Since = cut
	}
	if cmd.Flags().Changed("remote") {
		remote := flagBool(cmd, "remote")
		q.Remote = &remote
	}
	if lane := flagStr(cmd, "lane"); lane != "" {
		lanes, err := s.LoadLanes()
		if err != nil {
			return err
		}
		var names []string
		for k := range lanes {
			names = append(names, k)
		}
		if _, ok := lanes[strings.ToLower(lane)]; !ok {
			sort.Strings(names)
			return fmt.Errorf("unknown lane %q; available: %s", lane, strings.Join(names, ", "))
		}
		q.Lane = lane
		q.Lanes = lanes
	}
	out := roles.Filter(all, q)

	if wantJSON(cmd) {
		// Omit the heavy description field in list output.
		lite := make([]model.Role, len(out))
		copy(lite, out)
		for i := range lite {
			lite[i].Description = ""
		}
		return emitJSON(lite)
	}
	if len(out) == 0 {
		info("no roles match")
		return nil
	}
	tw := newTabWriter()
	fmt.Fprintln(tw, "ID\tTITLE\tEMPLOYER\tLOCATION\tREMOTE\tSALARY\tSTATUS")
	for _, r := range out {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.GlobalID, truncate(r.Title, 40), truncate(r.Employer, 20),
			truncate(r.Location, 22), yesno(r.Remote), truncate(r.Salary, 18), r.Status)
	}
	return tw.Flush()
}

// flagStr returns the named string flag's value, or "" when the flag is not
// defined on cmd (the bare group carries no filter flags).
func flagStr(cmd *cobra.Command, name string) string {
	if f := cmd.Flags().Lookup(name); f != nil {
		v, _ := cmd.Flags().GetString(name)
		return v
	}
	return ""
}

// flagBool returns the named bool flag's value, or false when it is not defined.
func flagBool(cmd *cobra.Command, name string) bool {
	if f := cmd.Flags().Lookup(name); f != nil {
		v, _ := cmd.Flags().GetBool(name)
		return v
	}
	return false
}

func newRolesChangesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "changes",
		Short: "Show the delta from the last import",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore(cmd)
			if err != nil {
				return err
			}
			var res roles.ImportResult
			found, err := s.ReadJSON(roles.LastImportRelPath, &res)
			if err != nil {
				return err
			}
			if !found {
				info("no imports yet")
				return nil
			}
			if wantJSON(cmd) {
				return emitJSON(res)
			}
			all, _ := s.LoadRoles()
			newN, changedN, goneN := res.Counts()
			info("Last import (%s): %d new, %d changed, %d gone", res.Company, newN, changedN, goneN)
			printRoleGroup(all, "new", res.New)
			printRoleGroup(all, "changed", res.Changed)
			printRoleGroup(all, "gone", res.Gone)
			return nil
		},
	}
}

func newRolesGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "show <id>",
		Aliases: []string{"get"},
		Short:   "Show one role's full detail (including description)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore(cmd)
			if err != nil {
				return err
			}
			all, err := s.LoadRoles()
			if err != nil {
				return err
			}
			r, ok := roles.Find(all, args[0])
			if !ok {
				return fmt.Errorf("no role matching id %q", args[0])
			}
			if wantJSON(cmd) {
				return emitJSON(r)
			}
			fmt.Printf("%s\n%s  %s\n%s | remote=%t | %s | %s\n%s\n\n%s\n",
				r.Title, r.Employer, r.GlobalID, r.Location, r.Remote, r.Salary, r.Status, r.URL, r.Description)
			return nil
		},
	}
}

func newRolesRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "Drop a junk role from the index by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			all, err := s.LoadRoles()
			if err != nil {
				return err
			}
			r, ok := roles.Find(all, args[0])
			if !ok {
				return fmt.Errorf("no role matching id %q", args[0])
			}
			out := all[:0]
			for _, x := range all {
				if x.GlobalID == r.GlobalID {
					continue
				}
				out = append(out, x)
			}
			if err := s.SaveRoles(out); err != nil {
				return err
			}
			if wantJSON(cmd) {
				return emitJSON(r)
			}
			info("Removed role %s (%s  %s)", r.GlobalID, r.Employer, r.Title)
			return nil
		},
	}
}

// readInput returns the payload from the file arg, or stdin when the arg is "-"
// or absent.
func readInput(args []string) ([]byte, error) {
	if len(args) == 0 || args[0] == "-" {
		// Guard against hanging on an interactive terminal with nothing piped in.
		if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
			return nil, fmt.Errorf("no input: pass a file path or pipe JSON in (e.g. jobhive scrape ... | jl role import -)")
		}
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(args[0])
}

func printRoleGroup(all []model.Role, label string, ids []string) {
	for _, id := range ids {
		if r, ok := roles.Find(all, id); ok {
			info("  %-8s %s  %s", label+":", r.Employer, r.Title)
		} else {
			info("  %-8s %s", label+":", id)
		}
	}
}

func yesno(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
