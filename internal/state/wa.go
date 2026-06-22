package state

import "github.com/bttnns/joblog/internal/model"

// Washington (ESD).
//
// Source: Washington Employment Security Department, job-search requirements
// and the ESD job-search log.
// https://esd.wa.gov/unemployment/job-search-requirements
//
// Washington requires 3 job-search activities per week. The qualifying set is
// broad: alongside applications and interviews, ESD counts activities such as
// watching career videos or completing online courses (logged as "other" with
// a note here, so we count "other" for Washington). Keep the ESD job-search log
// for 30 days past the end of the benefit year (not submitted weekly).
type wa struct{}

func (wa) Code() string      { return "wa" }
func (wa) Name() string      { return "Washington" }
func (wa) MinDefault() int   { return 3 }
func (wa) Submit() bool      { return false }
func (wa) FormName() string  { return "ESD job-search log" }
func (wa) Retention() string { return "30 days past the benefit year" }
func (wa) SourceURL() string { return "https://esd.wa.gov/unemployment/job-search-requirements" }

// waQualify is broader than the default: Washington counts a wide range of
// reemployment activities, including ones logged as "other" (career videos,
// online courses). A workforce-office (WorkSource) visit also counts.
func waQualify(e model.Entry) bool { return true }

func (wa) Check(week []model.Entry, min int) (int, bool) {
	n := countQualifying(week, waQualify)
	return n, meets(n, min)
}

func (w wa) Render(week []model.Entry) string {
	return renderGeneric(w, week, w.MinDefault(), waQualify)
}

func init() { register(wa{}) }
