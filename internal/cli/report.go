package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/bttnns/joblog/internal/dates"
	"github.com/bttnns/joblog/internal/model"
	"github.com/bttnns/joblog/internal/state"
	"github.com/bttnns/joblog/internal/store"
	"github.com/spf13/cobra"
)

func init() { addCommand(newReportCmd) }

// ErrShortOfMinimum is returned by report --check when the week is short. main
// maps it to a distinct exit code so a script can tell "ran fine, you are behind"
// apart from "the tool failed".
var ErrShortOfMinimum = errors.New("short of the weekly work-search minimum")

func newReportCmd() *cobra.Command {
	var stateCode, week string
	var check bool

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Render this week's state work-search report and compliance status",
		Long: "Render the active state's weekly work-search report from the log. Defaults to\n" +
			"the current week. --check returns only a compliance status and a nonzero exit\n" +
			"code when you are short, so it can gate a script.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore(cmd)
			if err != nil {
				return err
			}
			cfg, err := s.LoadConfig()
			if err != nil {
				return err
			}
			code := firstNonEmpty(stateCode, cfg.State)
			if code == "" {
				return fmt.Errorf("no state set; use --state <code> or: jl config set state <code> (states: %s)", joinVocab(state.Codes()))
			}
			p, ok := state.Get(code)
			if !ok {
				return fmt.Errorf("unknown state %q (states: %s)", code, joinVocab(state.Codes()))
			}

			ws, err := dates.ParseWeek(nowFunc(), week)
			if err != nil {
				return err
			}
			weekEntries, err := entriesInWeek(s, ws)
			if err != nil {
				return err
			}

			min := p.MinDefault()
			if cfg.Min > 0 {
				min = cfg.Min
			}
			n, compliant := p.Check(weekEntries, min)

			if check {
				if wantJSON(cmd) {
					_ = emitJSON(complianceView(p, ws, n, min, compliant))
				} else {
					info("%s week of %s: %d/%d activities, compliant=%t", p.Code(), ws.Format(dates.ISO), n, min, compliant)
				}
				if !compliant {
					// Nonzero exit so a script can gate on it; the message is
					// already on stderr above (or JSON on stdout).
					return ErrShortOfMinimum
				}
				return nil
			}

			if wantJSON(cmd) {
				return emitJSON(map[string]any{
					"compliance": complianceView(p, ws, n, min, compliant),
					"render":     p.Render(weekEntries),
				})
			}
			fmt.Printf("State: %s (%s)\n", p.Name(), p.Code())
			fmt.Printf("Form: %s   Submit weekly: %t   Retention: %s\n", p.FormName(), p.Submit(), p.Retention())
			fmt.Printf("Required activities/week: %d   This week: %d   Compliant: %t\n\n", min, n, compliant)
			fmt.Print(p.Render(weekEntries))
			return nil
		},
	}
	cmd.Flags().StringVar(&stateCode, "state", "", "state code (default: configured state)")
	cmd.Flags().StringVar(&week, "week", "", "week to report (any YYYY-MM-DD in it; default current)")
	cmd.Flags().BoolVar(&check, "check", false, "print compliance status only; nonzero exit if short")

	cmd.AddCommand(newReportStatesCmd())
	return cmd
}

func newReportStatesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "states",
		Short: "List the supported states and their known requirements",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			all := state.All()
			if wantJSON(cmd) {
				out := make([]map[string]any, 0, len(all))
				for _, p := range all {
					out = append(out, stateView(p))
				}
				return emitJSON(out)
			}
			tw := newTabWriter()
			fmt.Fprintln(tw, "CODE\tSTATE\tMIN/WK\tSUBMIT\tFORM\tRETENTION")
			for _, p := range all {
				fmt.Fprintf(tw, "%s\t%s\t%d\t%t\t%s\t%s\n",
					p.Code(), p.Name(), p.MinDefault(), p.Submit(), p.FormName(), p.Retention())
			}
			tw.Flush()
			info("\nRequirements change and can vary by county; verify at each state's official source.")
			return nil
		},
	}
}

func entriesInWeek(s *store.Store, weekStart time.Time) ([]model.Entry, error) {
	log, err := s.LoadLog()
	if err != nil {
		return nil, err
	}
	var out []model.Entry
	for _, e := range log {
		if dates.InWeek(e.Date, weekStart) {
			out = append(out, e)
		}
	}
	return out, nil
}

func complianceView(p state.Profile, ws time.Time, n, min int, ok bool) map[string]any {
	return map[string]any{
		"state":      p.Code(),
		"week":       ws.Format(dates.ISO),
		"count":      n,
		"required":   min,
		"compliant":  ok,
		"submit":     p.Submit(),
		"form":       p.FormName(),
		"source_url": p.SourceURL(),
	}
}

func stateView(p state.Profile) map[string]any {
	return map[string]any{
		"code":       p.Code(),
		"name":       p.Name(),
		"min":        p.MinDefault(),
		"submit":     p.Submit(),
		"form":       p.FormName(),
		"retention":  p.Retention(),
		"source_url": p.SourceURL(),
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
