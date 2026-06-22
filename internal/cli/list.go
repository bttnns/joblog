package cli

import (
	"fmt"
	"sort"

	"github.com/bttnns/joblog/internal/dates"
	"github.com/bttnns/joblog/internal/model"
	"github.com/bttnns/joblog/internal/worklog"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List and filter work-search entries",
		Long: "List entries from the work-search log. Combine filters to slice the pipeline,\n" +
			"for example: jl log ls --status onsite, or jl log ls --since 7d.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogList(cmd)
		},
	}
	f := cmd.Flags()
	f.String("status", "", "filter by status")
	f.String("employer", "", "filter by employer (substring)")
	f.String("type", "", "filter by activity type")
	f.String("week", "", "filter to a week (any YYYY-MM-DD in it; default off)")
	f.String("since", "", "only entries on/after a date or window (e.g. 2026-03-16, 7d, 2w)")
	return cmd
}

// runLogList renders the work-search log, reading any filter flags from cmd. It
// is shared by `jl log ls` and the bare `jl log` group (which carries no filter
// flags and so lists everything).
func runLogList(cmd *cobra.Command) error {
	s, err := openStore(cmd)
	if err != nil {
		return err
	}
	log, err := s.LoadLog()
	if err != nil {
		return err
	}

	q := worklog.Query{
		Status:   flagStr(cmd, "status"),
		Type:     flagStr(cmd, "type"),
		Employer: flagStr(cmd, "employer"),
	}
	if week := flagStr(cmd, "week"); week != "" {
		ws, err := dates.ParseWeek(nowFunc(), week)
		if err != nil {
			return err
		}
		q.Week = ws
	}
	if since := flagStr(cmd, "since"); since != "" {
		cut, err := dates.ParseSince(nowFunc(), since)
		if err != nil {
			return err
		}
		q.Since = cut
	}
	out := worklog.Filter(log, q)

	sort.SliceStable(out, func(i, j int) bool { return out[i].Date < out[j].Date })

	if wantJSON(cmd) {
		return emitJSON(out)
	}
	if len(out) == 0 {
		info("no entries match")
		return nil
	}
	tw := newTabWriter()
	fmt.Fprintln(tw, "ID\tDATE\tTYPE\tSTATUS\tEMPLOYER\tTITLE")
	for _, e := range out {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			e.ID, e.Date, e.Type, e.Status, truncate(e.Employer, 28), truncate(e.Title, 40))
	}
	return tw.Flush()
}

// printLogEntry prints one entry's full detail for `jl log show`.
func printLogEntry(e model.Entry) {
	fmt.Printf("%s  %s\n", e.ID, e.Date)
	fmt.Printf("  Type:     %s\n", e.Type)
	fmt.Printf("  Status:   %s\n", e.Status)
	fmt.Printf("  Employer: %s\n", e.Employer)
	if e.Company != "" {
		fmt.Printf("  Company:  %s\n", e.Company)
	}
	fmt.Printf("  Title:    %s\n", e.Title)
	if e.JobType != "" {
		fmt.Printf("  Job type: %s\n", e.JobType)
	}
	if e.URL != "" {
		fmt.Printf("  URL:      %s\n", e.URL)
	}
	if e.Method != "" {
		fmt.Printf("  Method:   %s\n", e.Method)
	}
	if e.Contact != "" {
		fmt.Printf("  Contact:  %s\n", e.Contact)
	}
	if e.Resume != "" {
		fmt.Printf("  Resume:   %s\n", e.Resume)
	}
	if e.Notes != "" {
		fmt.Printf("  Notes:    %s\n", e.Notes)
	}
}

// truncate shortens s to at most n runes, appending an ellipsis when it cuts.
// It counts runes, not bytes, so a multibyte employer or title name (accents,
// CJK) is never sliced mid-rune into a replacement character.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
