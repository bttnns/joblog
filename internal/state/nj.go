package state

import "github.com/bttnns/joblog/internal/model"

// New Jersey (NJDOL).
//
// Source: New Jersey Department of Labor and Workforce Development, work-search
// guidance and form BC-514 (Record of Job Contacts).
// https://www.nj.gov/labor/myunemployment/before/about/howtoapply/workregistration.shtml
//
// New Jersey expects a reasonable effort, generally 3 employer contacts per
// week per BC-514 guidance (this is the agency's guidance, not a hard number
// from N.J.A.C. 12:17-4.3). Keep the BC-514 record for the life of the claim
// (not submitted weekly).
type nj struct{}

func (nj) Code() string      { return "nj" }
func (nj) Name() string      { return "New Jersey" }
func (nj) MinDefault() int   { return 3 } // BC-514 guidance, not a regulatory hard number
func (nj) Submit() bool      { return false }
func (nj) FormName() string  { return "BC-514 Record of Job Contacts" }
func (nj) Retention() string { return "the life of the claim" }
func (nj) SourceURL() string {
	return "https://www.nj.gov/labor/myunemployment/before/about/howtoapply/workregistration.shtml"
}

func (nj) Check(week []model.Entry, min int) (int, bool) { return checkDefault(week, min) }

func (n nj) Render(week []model.Entry) string {
	return renderGeneric(n, week, n.MinDefault(), defaultQualify)
}

func init() { register(nj{}) }
