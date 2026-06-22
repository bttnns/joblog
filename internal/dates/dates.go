// Package dates holds the small time helpers joblog needs: a --since parser
// that understands days and weeks (which time.ParseDuration does not) and week
// math for the weekly compliance report. Weeks start on Monday.
package dates

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ISO is the only date format joblog reads or writes.
const ISO = "2006-01-02"

// Today returns today's date as an ISO string in local time.
func Today() string { return time.Now().Format(ISO) }

// ParseSince interprets a --since value, relative to now. It accepts either an
// absolute ISO date ("2026-03-16") or a duration with day/week units ("7d",
// "2w") as well as the standard Go units ("24h"). It returns the cutoff time;
// callers keep entries on or after it.
func ParseSince(now time.Time, s string) (time.Time, error) {
	if t, err := time.Parse(ISO, s); err == nil {
		return t, nil
	}
	d, err := parseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --since %q: want a date (2006-01-02) or duration (7d, 2w, 24h)", s)
	}
	return now.Add(-d), nil
}

// parseDuration extends time.ParseDuration with d (days) and w (weeks). Day and
// week counts must be non-negative: a --since window reaches into the past, so a
// negative count (which would set a cutoff in the future and silently match
// nothing) is rejected rather than honored.
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if n, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.Atoi(n)
		if err != nil {
			return 0, err
		}
		if days < 0 {
			return 0, fmt.Errorf("day count cannot be negative")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	if n, ok := strings.CutSuffix(s, "w"); ok {
		weeks, err := strconv.Atoi(n)
		if err != nil {
			return 0, err
		}
		if weeks < 0 {
			return 0, fmt.Errorf("week count cannot be negative")
		}
		return time.Duration(weeks) * 7 * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// OnOrAfter reports whether the ISO date dateStr falls on or after cutoff,
// compared at calendar-day granularity (cutoff's clock time is ignored). This is
// the shared meaning of --since across the log and role listings, so "7d" means
// the same thing in both. An unparseable date is treated as not matching.
//
// Both dates are interpreted in cutoff's location, so the comparison is a true
// calendar-day comparison. Parsing the stored date as UTC while cutoff is local
// would make an entry stamped "today" fall a few hours before a local-midnight
// cutoff and be wrongly excluded for any user west of UTC.
func OnOrAfter(dateStr string, cutoff time.Time) bool {
	t, err := time.ParseInLocation(ISO, dateStr, cutoff.Location())
	if err != nil {
		return false
	}
	cut := time.Date(cutoff.Year(), cutoff.Month(), cutoff.Day(), 0, 0, 0, 0, cutoff.Location())
	return !t.Before(cut)
}

// WeekStart returns midnight on the Monday of t's week.
func WeekStart(t time.Time) time.Time {
	offset := (int(t.Weekday()) + 6) % 7 // Monday=0 .. Sunday=6
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location()).AddDate(0, 0, -offset)
}

// ParseWeek resolves a --week value to that week's Monday. An empty value means
// the current week. A value may be any ISO date within the wanted week.
func ParseWeek(now time.Time, week string) (time.Time, error) {
	if week == "" {
		return WeekStart(now), nil
	}
	t, err := time.Parse(ISO, week)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --week %q: want a date (2006-01-02)", week)
	}
	return WeekStart(t), nil
}

// InWeek reports whether the ISO date dateStr falls in the 7-day window that
// starts at weekStart. The date is interpreted in weekStart's location so the
// comparison is a true calendar-day comparison (see OnOrAfter for why this
// matters west of UTC).
func InWeek(dateStr string, weekStart time.Time) bool {
	t, err := time.ParseInLocation(ISO, dateStr, weekStart.Location())
	if err != nil {
		return false
	}
	return !t.Before(weekStart) && t.Before(weekStart.AddDate(0, 0, 7))
}
