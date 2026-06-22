package state

import "github.com/bttnns/joblog/internal/model"

// North Carolina (DES).
//
// Source: North Carolina Division of Employment Security, work-search
// requirements (MyNCUIBenefits).
// https://des.nc.gov/need-help/work-search-requirements
//
// North Carolina requires 3 work-search activities per week; one may be a
// reemployment activity (for example a NCWorks workshop). New claims enter
// activities online before weekly certification (submitted), so we treat the
// profile as submit. Keep records for up to 5 years.
//
// VOLATILE: the online-entry requirement rolled out statewide as of December
// 2025; legacy claimants may still keep records rather than submit. Re-verify
// the Submit flag if the rollout details change.
type nc struct{}

func (nc) Code() string      { return "nc" }
func (nc) Name() string      { return "North Carolina" }
func (nc) MinDefault() int   { return 3 } // 1 may be a reemployment activity
func (nc) Submit() bool      { return true }
func (nc) FormName() string  { return "MyNCUIBenefits online work-search entry" }
func (nc) Retention() string { return "up to 5 years" }
func (nc) SourceURL() string { return "https://des.nc.gov/need-help/work-search-requirements" }

func (nc) Check(week []model.Entry, min int) (int, bool) { return checkDefault(week, min) }

func (n nc) Render(week []model.Entry) string {
	return renderGeneric(n, week, n.MinDefault(), defaultQualify)
}

func init() { register(nc{}) }
