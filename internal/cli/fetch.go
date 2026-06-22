package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bttnns/joblog/internal/store"
	"github.com/spf13/cobra"
)

func init() { addCommand(newFetchCmd) }

// fetchResult is one company's outcome, emitted in --json mode.
type fetchResult struct {
	Name    string `json:"name"`
	New     int    `json:"new"`
	Changed int    `json:"changed"`
	Gone    int    `json:"gone"`
	Skipped bool   `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

func newFetchCmd() *cobra.Command {
	var noGone, force, paused bool
	cmd := &cobra.Command{
		Use:   "fetch [company...]",
		Short: "Scrape and import roles for your tracked companies",
		Long: "Run the configured scraper for each tracked company and import its roles,\n" +
			"printing a per-company delta. With no arguments it fetches every active\n" +
			"company; --paused includes paused companies too; with names it fetches just\n" +
			"those (active or paused). jl itself makes no network calls: the scraper\n" +
			"(jobhive by default) does the HTTP and jl ingests its JSON. Set a different\n" +
			"producer with 'jl config set scraper'.\n\n" +
			"Companies with a custom or empty ATS, or no slug, are skipped: scrape those by\n" +
			"hand and pipe the result, e.g.\n" +
			"  <curl ...> | jl role import - --company <name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			cfg, err := s.LoadConfig()
			if err != nil {
				return err
			}
			companies, err := s.LoadCompanies()
			if err != nil {
				return err
			}
			targets, err := resolveFetchTargets(companies, args, paused)
			if err != nil {
				return err
			}

			template := scraperTemplate(cfg)
			now := nowFunc()
			jsonOut := wantJSON(cmd)
			var results []fetchResult
			fetched, skipped, failed := 0, 0, 0

			for _, c := range targets {
				if c.ATS == "" || c.ATS == "custom" || c.Slug == "" {
					skipped++
					results = append(results, fetchResult{Name: c.Name, Skipped: true})
					if !jsonOut {
						info("%s: skipped (ATS=%q, slug=%q); scrape by hand and pipe in:\n  <curl ...> | jl role import - --company %s", c.Name, c.ATS, c.Slug, c.Name)
					}
					continue
				}
				payload, err := scrape(template, c.ATS, c.Slug)
				if err != nil {
					failed++
					results = append(results, fetchResult{Name: c.Name, Error: err.Error()})
					if !jsonOut {
						info("%s: scrape failed: %v", c.Name, err)
					}
					continue
				}
				res, _, err := importPayload(s, payload, c.Name, noGone, force, now)
				if err != nil {
					failed++
					results = append(results, fetchResult{Name: c.Name, Error: err.Error()})
					if !jsonOut {
						info("%s: import failed: %v", c.Name, err)
					}
					continue
				}
				fetched++
				newN, changedN, goneN := res.Counts()
				total := totalRolesForCompany(payload)
				results = append(results, fetchResult{Name: c.Name, New: newN, Changed: changedN, Gone: goneN})
				if !jsonOut {
					info("%s: %d roles (%d new, %d changed, %d gone)", c.Name, total, newN, changedN, goneN)
				}
			}

			if jsonOut {
				return emitJSON(results)
			}
			info("Fetched %d, skipped %d, failed %d.", fetched, skipped, failed)
			info("Review what is new with: jl role ls --new")
			return nil
		},
	}
	cmd.Flags().BoolVar(&noGone, "no-gone", false, "do not retire roles absent from a company's payload (for a partial scrape)")
	cmd.Flags().BoolVar(&force, "force", false, "apply even when an import would retire most of a company's open roles")
	cmd.Flags().BoolVar(&paused, "paused", false, "also fetch paused companies (default fetches only active ones)")
	return cmd
}

// resolveFetchTargets returns the companies to fetch: the named ones (erroring on
// an unknown name) regardless of status when args are given, else every active
// company (plus paused ones when includePaused is set).
func resolveFetchTargets(companies []store.Company, args []string, includePaused bool) ([]store.Company, error) {
	if len(args) == 0 {
		var out []store.Company
		for _, c := range companies {
			if includePaused || c.Status == store.StatusActive {
				out = append(out, c)
			}
		}
		return out, nil
	}
	var out []store.Company
	for _, name := range args {
		var found *store.Company
		for i := range companies {
			if strings.EqualFold(companies[i].Name, name) {
				found = &companies[i]
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("no company named %q (add it with: jl company add)", name)
		}
		out = append(out, *found)
	}
	return out, nil
}

// totalRolesForCompany counts the roles in a scraper payload for the delta line.
// A payload that does not parse as a JSON array reports zero; importPayload has
// already validated and surfaced any parse error before this is called.
func totalRolesForCompany(payload []byte) int {
	var arr []json.RawMessage
	if err := json.Unmarshal(payload, &arr); err != nil {
		return 0
	}
	return len(arr)
}
