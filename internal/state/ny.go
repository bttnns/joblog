package state

import (
	"fmt"

	"github.com/bttnns/joblog/internal/model"
)

// New York (NYSDOL).
//
// Source: New York State Department of Labor, work-search requirements and the
// WS-5 Record of Job Search Activities.
// https://dol.ny.gov/unemployment/job-search-record
//
// New York requires 3 work-search activities per week, and they must be on
// three different days. Keep the WS-5 record for 1 year (not submitted weekly).
type ny struct{}

func (ny) Code() string      { return "ny" }
func (ny) Name() string      { return "New York" }
func (ny) MinDefault() int   { return 3 }
func (ny) Submit() bool      { return false }
func (ny) FormName() string  { return "WS-5 Record of Job Search Activities" }
func (ny) Retention() string { return "1 year" }
func (ny) SourceURL() string { return "https://dol.ny.gov/unemployment/job-search-record" }

// Check counts qualifying activities for display but requires them to fall on at
// least min DIFFERENT days, so several activities on one day do not over-credit
// the week.
func (ny) Check(week []model.Entry, min int) (int, bool) {
	n := countQualifying(week, defaultQualify)
	return n, meets(countDistinctDays(week, defaultQualify), min)
}

func (n ny) Render(week []model.Entry) string {
	return renderGeneric(n, week, n.MinDefault(), defaultQualify)
}

// explain surfaces the distinct-days rule, since the qualifying count alone can
// look sufficient while the week still falls short on spread.
func (ny) explain(week []model.Entry, min int) string {
	days := countDistinctDays(week, defaultQualify)
	return fmt.Sprintf("Rule: the activities must fall on %d different days; you have %d.", min, days)
}

func init() { register(ny{}) }
