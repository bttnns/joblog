package state

import "github.com/bttnns/joblog/internal/model"

// Florida (DEO / CONNECT).
//
// Source: Florida Department of Commerce (Reemployment Assistance),
// work-search requirements and the Work Search Record.
// https://www.floridajobs.org/Reemployment-Assistance-Service-Center/reemployment-assistance/claimants
//
// Florida requires 5 work-search contacts per week (3 in small counties of
// 75,000 or fewer residents). Activities are submitted weekly through CONNECT.
// Keep the Work Search Record for 1 year. Register with Employ Florida.
type fl struct{}

func (fl) Code() string      { return "fl" }
func (fl) Name() string      { return "Florida" }
func (fl) MinDefault() int   { return 5 }
func (fl) Submit() bool      { return true }
func (fl) FormName() string  { return "Work Search Record (CONNECT)" }
func (fl) Retention() string { return "1 year" }
func (fl) SourceURL() string {
	return "https://www.floridajobs.org/Reemployment-Assistance-Service-Center/reemployment-assistance/claimants"
}

func (fl) Check(week []model.Entry, min int) (int, bool) { return checkDefault(week, min) }

func (f fl) Render(week []model.Entry) string {
	return renderGeneric(f, week, f.MinDefault(), defaultQualify)
}

func init() { register(fl{}) }
