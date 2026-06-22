package state

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/bttnns/joblog/internal/model"
)

// weekRange returns the earliest and latest ISO date among the entries, for the
// report header. When the slice is empty it returns empty strings. Dates are
// already ISO strings (YYYY-MM-DD), which sort lexically, so no parsing needed.
func weekRange(week []model.Entry) (start, end string) {
	for _, e := range week {
		if e.Date == "" {
			continue
		}
		if start == "" || e.Date < start {
			start = e.Date
		}
		if end == "" || e.Date > end {
			end = e.Date
		}
	}
	return start, end
}

// dash returns v, or a placeholder when v is empty, so columns never collapse.
func dash(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

// disclaimer is the mandatory closing line every Render emits.
func disclaimer(sourceURL string) string {
	return "Requirements change and can vary by county; verify at " + sourceURL + "."
}

// sortedWeek returns the qualifying entries in date order so renders are
// deterministic (and golden files stable) regardless of log insertion order.
func sortedWeek(week []model.Entry, qualify func(model.Entry) bool) []model.Entry {
	out := make([]model.Entry, 0, len(week))
	for _, e := range week {
		if qualify(e) {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].Employer < out[j].Employer
	})
	return out
}

// explainer is an optional Profile capability. A state whose rule is more
// nuanced than "N qualifying activities" (NY's distinct-days, PA's compound
// 2+1) implements it to add a one-line explanation to the header, so a user who
// sees a high activity count but a BELOW status understands the actual rule.
type explainer interface {
	explain(week []model.Entry, min int) string
}

// header builds the shared report banner: state, form, week span, and the
// count versus the required minimum.
func header(b *strings.Builder, p Profile, week []model.Entry, min int) {
	n, ok := p.Check(week, min)
	start, end := weekRange(week)
	span := "(no dated activities)"
	if start != "" {
		if start == end {
			span = start
		} else {
			span = start + " to " + end
		}
	}
	required := "reasonable / unspecified"
	if min > 0 {
		required = fmt.Sprintf("%d", min)
	}
	status := "MEETS requirement"
	if !ok {
		status = "BELOW requirement"
	}
	submit := "keep for audit"
	if p.Submit() {
		submit = "submit weekly to the agency"
	}

	fmt.Fprintf(b, "%s Weekly Work Search Log\n", p.Name())
	fmt.Fprintf(b, "Form: %s\n", p.FormName())
	fmt.Fprintf(b, "Week: %s\n", span)
	fmt.Fprintf(b, "Activities: %d qualifying / %s required (%s)\n", n, required, status)
	if ex, ok := p.(explainer); ok {
		if line := ex.explain(week, min); line != "" {
			fmt.Fprintf(b, "%s\n", line)
		}
	}
	fmt.Fprintf(b, "Filing: %s. Retention: %s.\n", submit, p.Retention())
}

// renderGeneric is the default weekly log used by every state except TX. It
// prints the shared header, a table of qualifying activities, then the
// disclaimer. qualify selects which entries appear (it should match the state's
// Check rule).
func renderGeneric(p Profile, week []model.Entry, min int, qualify func(model.Entry) bool) string {
	var b strings.Builder
	header(&b, p, week, min)
	b.WriteString("\n")

	rows := sortedWeek(week, qualify)
	if len(rows) == 0 {
		b.WriteString("No qualifying activities recorded for this week.\n")
	} else {
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "DATE\tACTIVITY\tEMPLOYER\tJOB TYPE\tMETHOD\tCONTACT\tRESULT\tURL")
		for _, e := range rows {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				dash(e.Date), dash(e.Type), dash(e.Employer), dash(e.JobType),
				dash(e.Method), dash(e.Contact), dash(e.Status), dash(e.URL))
		}
		tw.Flush()
	}

	b.WriteString("\n")
	b.WriteString(disclaimer(p.SourceURL()))
	b.WriteString("\n")
	return b.String()
}
