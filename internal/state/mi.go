package state

import "github.com/bttnns/joblog/internal/model"

// Michigan (UIA / MiWAM).
//
// Source: Michigan Unemployment Insurance Agency, work-search requirements
// (entered in MiWAM).
// https://www.michigan.gov/uia/claimants/registration-and-seeking-work
//
// Michigan currently requires 1 work-search activity per week, entered directly
// in MiWAM (there is no separate claimant form); missing it can block payment,
// so we treat the profile as submit. Keep records for 2 years.
//
// VOLATILE: the weekly minimum is scheduled to rise to 3 in July 2026.
// Re-verify MinDefault after that date.
type mi struct{}

func (mi) Code() string      { return "mi" }
func (mi) Name() string      { return "Michigan" }
func (mi) MinDefault() int   { return 1 } // rises to 3 in July 2026 (re-verify)
func (mi) Submit() bool      { return true }
func (mi) FormName() string  { return "MiWAM work-search entry (no claimant form)" }
func (mi) Retention() string { return "2 years" }
func (mi) SourceURL() string {
	return "https://www.michigan.gov/uia/claimants/registration-and-seeking-work"
}

func (mi) Check(week []model.Entry, min int) (int, bool) { return checkDefault(week, min) }

func (m mi) Render(week []model.Entry) string {
	return renderGeneric(m, week, m.MinDefault(), defaultQualify)
}

func init() { register(mi{}) }
