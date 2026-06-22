// Package worklog queries the work-search log. It is pure: no file IO and no
// network. The CLI loads entries via the store and calls Filter, mirroring the
// roles package so the two record types are queried the same way (and the log
// filter is unit-testable, not buried in a command handler).
package worklog

import (
	"strings"
	"time"

	"github.com/bttnns/joblog/internal/dates"
	"github.com/bttnns/joblog/internal/model"
)

// Query filters the work-search log. Zero-value fields mean "no constraint".
type Query struct {
	Status   string    // exact match on Status
	Type     string    // exact match on Type
	Employer string    // case-insensitive substring match on Employer
	Week     time.Time // keep entries within this week (Monday start); zero = no bound
	Since    time.Time // keep entries on/after this day; zero = no bound
}

// Filter returns the entries matching q. Filters combine with AND. The returned
// slice is fresh; the input is not modified and its order is preserved.
func Filter(entries []model.Entry, q Query) []model.Entry {
	hasWeek := !q.Week.IsZero()
	hasSince := !q.Since.IsZero()
	out := make([]model.Entry, 0, len(entries))
	for _, e := range entries {
		if q.Status != "" && e.Status != q.Status {
			continue
		}
		if q.Type != "" && e.Type != q.Type {
			continue
		}
		if q.Employer != "" && !strings.Contains(strings.ToLower(e.Employer), strings.ToLower(q.Employer)) {
			continue
		}
		if hasWeek && !dates.InWeek(e.Date, q.Week) {
			continue
		}
		if hasSince && !dates.OnOrAfter(e.Date, q.Since) {
			continue
		}
		out = append(out, e)
	}
	return out
}
