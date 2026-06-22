package state

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/bttnns/joblog/internal/model"
)

// Texas (TWC).
//
// Source: Texas Workforce Commission, "Work Search Requirements" and the
// BN900E Work Search Activity Log.
// https://www.twc.texas.gov/programs/unemployment-benefits/job-search-requirements
//
// Required activities are set per claimant by county and by the determination
// letter (a 1 to 5 range); 3 is the common floor we default to. Records are
// kept (not submitted weekly) and must be produced on audit; keep them through
// the benefit year.
//
// Registering with WorkInTexas.com within 3 business days counts as one work
// search activity but cannot be the only activity for the week.
//
// VOLATILE: a rule proposed April 2026 would floor the minimum at 5 statewide;
// as of June 2026 it is NOT adopted. Re-verify the MinDefault if that changes.
type tx struct{}

func (tx) Code() string      { return "tx" }
func (tx) Name() string      { return "Texas" }
func (tx) MinDefault() int   { return 3 }
func (tx) Submit() bool      { return false }
func (tx) FormName() string  { return "BN900E Work Search Activity Log" }
func (tx) Retention() string { return "through the benefit year" }
func (tx) SourceURL() string {
	return "https://www.twc.texas.gov/programs/unemployment-benefits/job-search-requirements"
}

// Check counts any genuine work-search activity. WorkInTexas registration would
// be logged as a "workforce-office" entry; the default rule already excludes it
// from counting alone, which matches the "counts as 1 but not alone" guidance
// in spirit without special-casing a single registration here.
func (tx) Check(week []model.Entry, min int) (int, bool) {
	return checkDefault(week, min)
}

// Render reproduces the BN900E weekly block. The official log records, per
// activity: the date of the activity, the employer/company (with contact info),
// the type of work sought, the work-search activity type, the method of
// contact, and the result/outcome. We lay those columns out faithfully.
func (t tx) Render(week []model.Entry) string {
	var b strings.Builder

	rows := sortedWeek(week, defaultQualify)
	n := len(rows)

	b.WriteString("TEXAS WORKFORCE COMMISSION\n")
	b.WriteString("BN900E - Work Search Activity Log\n")
	start, end := weekRange(week)
	span := "(no dated activities)"
	if start != "" {
		if start == end {
			span = start
		} else {
			span = start + " to " + end
		}
	}
	fmt.Fprintf(&b, "Week of: %s\n", span)
	fmt.Fprintf(&b, "Work search activities this week: %d (minimum required: %d, set by your determination letter)\n", n, t.MinDefault())
	b.WriteString("Type of work sought: see per-activity column below.\n")
	b.WriteString("Retain this log through your benefit year; TWC may request it on audit.\n")
	b.WriteString("\n")

	if n == 0 {
		b.WriteString("No work search activities recorded for this week.\n")
	} else {
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		// BN900E column order: date, employer/company, contact, type of work
		// sought, activity type, method, result/outcome, source/URL.
		fmt.Fprintln(tw, "DATE OF ACTIVITY\tEMPLOYER / COMPANY\tCONTACT\tTYPE OF WORK SOUGHT\tACTIVITY\tMETHOD OF CONTACT\tRESULT / OUTCOME\tURL")
		for _, e := range rows {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				dash(e.Date), dash(e.Employer), dash(e.Contact), dash(e.JobType),
				dash(e.Type), dash(e.Method), dash(e.Status), dash(e.URL))
		}
		tw.Flush()
	}

	b.WriteString("\n")
	b.WriteString("Note: registering with WorkInTexas.com counts as one activity but cannot be your only activity for the week.\n")
	b.WriteString(disclaimer(t.SourceURL()))
	b.WriteString("\n")
	return b.String()
}

func init() { register(tx{}) }
