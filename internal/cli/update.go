package cli

import (
	"fmt"

	"github.com/bttnns/joblog/internal/model"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var set model.Entry

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Edit fields on a log entry (for example advance --status)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			log, err := s.LoadLog()
			if err != nil {
				return err
			}
			idx, err := findEntry(log, args[0])
			if err != nil {
				return err
			}

			f := cmd.Flags()
			e := &log[idx]
			applyIfSet(f, "type", set.Type, &e.Type)
			applyIfSet(f, "employer", set.Employer, &e.Employer)
			applyIfSet(f, "title", set.Title, &e.Title)
			applyIfSet(f, "job-type", set.JobType, &e.JobType)
			applyIfSet(f, "url", set.URL, &e.URL)
			applyIfSet(f, "method", set.Method, &e.Method)
			applyIfSet(f, "status", set.Status, &e.Status)
			applyIfSet(f, "contact", set.Contact, &e.Contact)
			applyIfSet(f, "notes", set.Notes, &e.Notes)
			applyIfSet(f, "date", set.Date, &e.Date)

			if err := validateEntry(*e); err != nil {
				return err
			}
			if err := s.SaveLog(log); err != nil {
				return err
			}
			if wantJSON(cmd) {
				return emitJSON(*e)
			}
			info("Updated %s", e.ID)
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&set.Type, "type", "", "activity type ("+joinVocab(model.EntryTypes)+")")
	f.StringVar(&set.Employer, "employer", "", "employer name")
	f.StringVar(&set.Title, "title", "", "role title")
	f.StringVar(&set.JobType, "job-type", "", "type of work sought")
	f.StringVar(&set.URL, "url", "", "posting URL")
	f.StringVar(&set.Method, "method", "", "method ("+joinVocab(model.Methods)+")")
	f.StringVar(&set.Status, "status", "", "status ("+joinVocab(model.Statuses)+")")
	f.StringVar(&set.Contact, "contact", "", "contact name")
	f.StringVar(&set.Notes, "notes", "", "free-form notes")
	f.StringVar(&set.Date, "date", "", "activity date (YYYY-MM-DD)")
	return cmd
}

func applyIfSet(f interface{ Changed(string) bool }, name, val string, dst *string) {
	if f.Changed(name) {
		*dst = val
	}
}

func findEntry(log []model.Entry, idOrPrefix string) (int, error) {
	match := -1
	for i, e := range log {
		if e.ID == idOrPrefix {
			return i, nil
		}
		if len(idOrPrefix) > 0 && len(e.ID) >= len(idOrPrefix) && e.ID[:len(idOrPrefix)] == idOrPrefix {
			if match >= 0 {
				return -1, fmt.Errorf("id %q is ambiguous", idOrPrefix)
			}
			match = i
		}
	}
	if match < 0 {
		return -1, fmt.Errorf("no entry matching id %q", idOrPrefix)
	}
	return match, nil
}
