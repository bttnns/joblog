package state

import (
	"fmt"

	"github.com/bttnns/joblog/internal/model"
)

// Pennsylvania (PA UC).
//
// Source: Pennsylvania Department of Labor & Industry, Office of UC, work-search
// requirements and form UC-304.
// https://www.pa.gov/agencies/dli/programs-services/unemployment-compensation/uc-benefits/work-search-requirements.html
//
// Pennsylvania's weekly requirement is compound: at least 2 applications (or
// interviews) AND at least 1 additional qualifying activity (for example
// attending a job fair, networking, or a CareerLink workshop). Register with
// PA CareerLink within 30 days. Keep form UC-304 for 2 years (not submitted
// weekly).
//
// The 2 + 1 rule allows substitution at the higher tiers, but the floor we
// enforce is the documented 2 applications plus 1 other activity.
type pa struct{}

func (pa) Code() string      { return "pa" }
func (pa) Name() string      { return "Pennsylvania" }
func (pa) MinDefault() int   { return 3 } // 2 applications + 1 other activity
func (pa) Submit() bool      { return false }
func (pa) FormName() string  { return "UC-304 Record of Job Applications and Work Search" }
func (pa) Retention() string { return "2 years" }
func (pa) SourceURL() string {
	return "https://www.pa.gov/agencies/dli/programs-services/unemployment-compensation/uc-benefits/work-search-requirements.html"
}

// Check enforces the compound rule. n is the total of qualifying activities; ok
// requires at least 2 applications/interviews plus at least 1 other qualifying
// activity (and, if a custom min is set, n >= min as well).
func (pa) Check(week []model.Entry, min int) (int, bool) {
	apps, other := 0, 0
	for _, e := range week {
		switch {
		case isApplication(e):
			apps++
		case defaultQualify(e):
			other++
		}
	}
	n := apps + other
	compoundOK := apps >= 2 && other >= 1
	return n, compoundOK && meets(n, min)
}

func (p pa) Render(week []model.Entry) string {
	return renderGeneric(p, week, p.MinDefault(), defaultQualify)
}

// explain surfaces the compound 2+1 rule, since a week can clear the total count
// yet still fail for lacking the 2 applications or the 1 other activity.
func (pa) explain(week []model.Entry, min int) string {
	apps, other := 0, 0
	for _, e := range week {
		switch {
		case isApplication(e):
			apps++
		case defaultQualify(e):
			other++
		}
	}
	return fmt.Sprintf("Rule: at least 2 applications + 1 other activity; you have %d applications and %d other.", apps, other)
}

func init() { register(pa{}) }
