package state

import "github.com/bttnns/joblog/internal/model"

// Georgia (GDOL).
//
// Source: Georgia Department of Labor, work-search requirements and form
// DOL-2798 (Work Search Record).
// https://dol.georgia.gov/individuals/unemployment-benefits
//
// Georgia requires 3 NEW employer contacts per week, recorded on form DOL-2798
// and submitted (via MyUI or fax). The qualifying unit is a contact with an
// employer (an application or interview), not a generic activity such as a
// workshop. Retention is unspecified by GDOL; keep records as long as practical.
type ga struct{}

func (ga) Code() string      { return "ga" }
func (ga) Name() string      { return "Georgia" }
func (ga) MinDefault() int   { return 3 } // 3 new employer contacts
func (ga) Submit() bool      { return true }
func (ga) FormName() string  { return "DOL-2798 Work Search Record" }
func (ga) Retention() string { return "unspecified (keep as long as practical)" }
func (ga) SourceURL() string { return "https://dol.georgia.gov/individuals/unemployment-benefits" }

// gaQualify counts new employer contacts only: a direct application or an
// interview. Networking, job fairs, and workforce-office visits do not count
// toward Georgia's new-contact requirement.
func gaQualify(e model.Entry) bool { return isApplication(e) }

// Check counts employer contacts for display but requires min DIFFERENT
// employers (by canonical slug), since Georgia wants new contacts, not repeated
// ones to the same employer.
func (ga) Check(week []model.Entry, min int) (int, bool) {
	n := countQualifying(week, gaQualify)
	return n, meets(countDistinctEmployers(week, gaQualify), min)
}

func (g ga) Render(week []model.Entry) string {
	return renderGeneric(g, week, g.MinDefault(), gaQualify)
}

func init() { register(ga{}) }
