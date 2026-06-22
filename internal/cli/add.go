package cli

import (
	"fmt"
	"path/filepath"

	"github.com/bttnns/joblog/internal/dates"
	"github.com/bttnns/joblog/internal/model"
	"github.com/bttnns/joblog/internal/roles"
	"github.com/bttnns/joblog/internal/store"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	var e model.Entry
	var fromRole string
	var resumeID string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add an entry to the work-search log (an application or any activity)",
		Long: "Add one entry to the work-search log. Applications and activities use the same\n" +
			"command; --status moves an application along over time. Use --from-role to\n" +
			"pre-fill employer, title, and url from a role in the index.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()

			if fromRole != "" {
				all, err := s.LoadRoles()
				if err != nil {
					return err
				}
				r, ok := roles.Find(all, fromRole)
				if !ok {
					return fmt.Errorf("no role matching id %q", fromRole)
				}
				if e.Employer == "" {
					e.Employer = r.Employer
				}
				if e.Title == "" {
					e.Title = r.Title
				}
				if e.URL == "" {
					e.URL = r.URL
				}
				if e.Company == "" {
					e.Company = r.Company
				}
				// Auto-link the role's tailored resume if one is stored, unless an
				// explicit --resume override is given below.
				if e.Resume == "" && resumeID == "" {
					if _, ok := tailoredResumeForRole(s, r); ok {
						e.Resume = r.GlobalID
					}
				}
			}

			// An explicit --resume overrides any auto-linked variant.
			if resumeID != "" {
				e.Resume = resumeID
			}

			// Stamp the canonical company slug at write-time so engagement is a
			// stable key match later, not a fuzzy compare on the employer string.
			if e.Company == "" {
				e.Company = model.Slug(e.Employer)
			}

			if e.Date == "" {
				e.Date = nowFunc().Format(dates.ISO)
			}
			if err := validateEntry(e); err != nil {
				return err
			}
			e.ID = storeNewID()

			log, err := s.LoadLog()
			if err != nil {
				return err
			}
			log = append(log, e)
			if err := s.SaveLog(log); err != nil {
				return err
			}

			if wantJSON(cmd) {
				return emitJSON(e)
			}
			if e.Resume != "" {
				info("Added %s: %s %s (%s) [resume: %s]", e.ID, e.Type, employerTitle(e), e.Status, e.Resume)
			} else {
				info("Added %s: %s %s (%s)", e.ID, e.Type, employerTitle(e), e.Status)
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&e.Type, "type", "applied", "activity type ("+joinVocab(model.EntryTypes)+")")
	f.StringVar(&e.Employer, "employer", "", "employer name")
	f.StringVar(&e.Company, "company", "", "canonical company slug linking this entry to a company (default: derived from --employer)")
	f.StringVar(&e.Title, "title", "", "role title")
	f.StringVar(&e.JobType, "job-type", "", "type of work sought (required by some state forms)")
	f.StringVar(&e.URL, "url", "", "posting URL")
	f.StringVar(&e.Method, "method", "", "method ("+joinVocab(model.Methods)+")")
	f.StringVar(&e.Status, "status", "applied", "status ("+joinVocab(model.Statuses)+")")
	f.StringVar(&e.Contact, "contact", "", "contact name")
	f.StringVar(&e.Notes, "notes", "", "free-form notes")
	f.StringVar(&e.Date, "date", "", "activity date (YYYY-MM-DD; default today)")
	f.StringVar(&fromRole, "from-role", "", "pre-fill from a role id (full global_id or unambiguous prefix); links its tailored resume if one exists")
	f.StringVar(&resumeID, "resume", "", "link a tailored resume to this entry (role id or path; overrides --from-role's auto-link)")
	return cmd
}

// tailoredResumeForRole reports whether a tailored resume variant is stored for
// the role and returns its source path (relative to the data dir).
func tailoredResumeForRole(s *store.Store, r model.Role) (string, bool) {
	slug := roleCompanySlug(r)
	matches, _ := filepath.Glob(s.Path(tailoredResumeGlob(slug, r.GlobalID)))
	for _, m := range matches {
		if filepath.Ext(m) == ".txt" {
			continue
		}
		return m, true
	}
	return "", false
}

// validateEntry rejects values outside the controlled vocabularies. Empty
// method is allowed (not every activity has one); type and status are required
// and default sensibly.
func validateEntry(e model.Entry) error {
	if !model.Valid(e.Type, model.EntryTypes) {
		return fmt.Errorf("invalid --type %q: want one of %s", e.Type, joinVocab(model.EntryTypes))
	}
	if !model.Valid(e.Status, model.Statuses) {
		return fmt.Errorf("invalid --status %q: want one of %s", e.Status, joinVocab(model.Statuses))
	}
	if e.Method != "" && !model.Valid(e.Method, model.Methods) {
		return fmt.Errorf("invalid --method %q: want one of %s", e.Method, joinVocab(model.Methods))
	}
	if !validDate(e.Date) {
		return fmt.Errorf("invalid --date %q: want YYYY-MM-DD", e.Date)
	}
	return nil
}

func employerTitle(e model.Entry) string {
	switch {
	case e.Employer != "" && e.Title != "":
		return e.Employer + " - " + e.Title
	case e.Employer != "":
		return e.Employer
	default:
		return e.Title
	}
}
