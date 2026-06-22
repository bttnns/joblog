package state

import "github.com/bttnns/joblog/internal/model"

// Illinois (IDES).
//
// Source: Illinois Department of Employment Security, work-search requirements
// and form ADJ034F (Record of Work Search).
// https://ides.illinois.gov/unemployment/resources/work-search-requirements.html
//
// Illinois sets no single fixed number; the standard is a systematic and
// sustained effort with "multiple" employer contacts per week (the TRA program
// specifies 5). Passively browsing a job board does NOT count as a work-search
// activity; only active contacts and activities do. Keep form ADJ034F for 53
// weeks (not submitted weekly).
type il struct{}

func (il) Code() string      { return "il" }
func (il) Name() string      { return "Illinois" }
func (il) MinDefault() int   { return 0 } // "multiple"; not a fixed number (TRA = 5)
func (il) Submit() bool      { return false }
func (il) FormName() string  { return "ADJ034F Record of Work Search" }
func (il) Retention() string { return "53 weeks" }
func (il) SourceURL() string {
	return "https://ides.illinois.gov/unemployment/resources/work-search-requirements.html"
}

// ilQualify is the default rule (job-board browsing would be logged as "other"
// and is already excluded, matching the Illinois "no passive browsing" rule).
func ilQualify(e model.Entry) bool { return defaultQualify(e) }

func (il) Check(week []model.Entry, min int) (int, bool) {
	n := countQualifying(week, ilQualify)
	return n, meets(n, min)
}

func (i il) Render(week []model.Entry) string {
	return renderGeneric(i, week, i.MinDefault(), ilQualify)
}

func init() { register(il{}) }
