package state

import "github.com/bttnns/joblog/internal/model"

// California (EDD).
//
// Source: California Employment Development Department, "Look for Work" /
// reemployment activities guidance and the DE 429Z claimant notice.
// https://edd.ca.gov/en/unemployment/work-search-requirements/
//
// California sets no fixed weekly number; the requirement is a per-claimant
// notice (DE 429Z) directing a reasonable effort. There is no submitted form;
// keep your record. Title 22 record-retention runs to roughly five years.
// Register with CalJOBS.
type ca struct{}

func (ca) Code() string      { return "ca" }
func (ca) Name() string      { return "California" }
func (ca) MinDefault() int   { return 0 }
func (ca) Submit() bool      { return false }
func (ca) FormName() string  { return "DE 429Z reemployment activities notice" }
func (ca) Retention() string { return "about 5 years (Title 22)" }
func (ca) SourceURL() string { return "https://edd.ca.gov/en/unemployment/work-search-requirements/" }

func (ca) Check(week []model.Entry, min int) (int, bool) { return checkDefault(week, min) }

func (c ca) Render(week []model.Entry) string {
	return renderGeneric(c, week, c.MinDefault(), defaultQualify)
}

func init() { register(ca{}) }
