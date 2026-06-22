package state

import "github.com/bttnns/joblog/internal/model"

// Virginia (VEC).
//
// Source: Virginia Employment Commission, work-search requirements (VUIS / CSS)
// and the Work Search Record.
// https://www.vec.virginia.gov/find-a-job/job-seeker-services/work-search-requirements
//
// Virginia requires 2 employer contacts per week with two DIFFERENT employers.
// Contacts must reach a hiring authority; responding to a blind ad does not
// count. Activities are submitted through VUIS / Gov2Go CSS (a paper Work
// Search Record is filed every 4 weeks for phone claimants), so we treat the
// profile as submit. Keep the record for 1 year.
type va struct{}

func (va) Code() string      { return "va" }
func (va) Name() string      { return "Virginia" }
func (va) MinDefault() int   { return 2 } // two different employers; hiring-authority contact
func (va) Submit() bool      { return true }
func (va) FormName() string  { return "Work Search Record (VUIS / CSS)" }
func (va) Retention() string { return "1 year" }
func (va) SourceURL() string {
	return "https://www.vec.virginia.gov/find-a-job/job-seeker-services/work-search-requirements"
}

// vaQualify counts direct employer contacts (applications and interviews); a
// hiring-authority contact is one of these, not a generic activity.
func vaQualify(e model.Entry) bool { return isApplication(e) }

// Check counts qualifying employer contacts for display but requires min
// DIFFERENT employers (by canonical slug, honoring an overridden min), because
// Virginia wants contacts to distinct employers, not repeats to one.
func (va) Check(week []model.Entry, min int) (int, bool) {
	n := countQualifying(week, vaQualify)
	return n, meets(countDistinctEmployers(week, vaQualify), min)
}

func (v va) Render(week []model.Entry) string {
	return renderGeneric(v, week, v.MinDefault(), vaQualify)
}

func init() { register(va{}) }
