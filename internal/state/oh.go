package state

import "github.com/bttnns/joblog/internal/model"

// Ohio (ODJFS).
//
// Source: Ohio Department of Job and Family Services, work-search requirements.
// https://unemployment.ohio.gov/help-resources/work-search
//
// Ohio requires 2 work-search activities per week. There is no single official
// numbered form; a generic Work-Search Activities Log is kept (not submitted
// weekly). Retain records for 18 months.
type oh struct{}

func (oh) Code() string      { return "oh" }
func (oh) Name() string      { return "Ohio" }
func (oh) MinDefault() int   { return 2 }
func (oh) Submit() bool      { return false }
func (oh) FormName() string  { return "Work-Search Activities Log (generic)" }
func (oh) Retention() string { return "18 months" }
func (oh) SourceURL() string { return "https://unemployment.ohio.gov/help-resources/work-search" }

func (oh) Check(week []model.Entry, min int) (int, bool) { return checkDefault(week, min) }

func (o oh) Render(week []model.Entry) string {
	return renderGeneric(o, week, o.MinDefault(), defaultQualify)
}

func init() { register(oh{}) }
