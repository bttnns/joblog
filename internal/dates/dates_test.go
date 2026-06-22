package dates

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(ISO, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func TestParseSince(t *testing.T) {
	now := mustParse(t, "2026-06-19")
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"2026-03-16", "2026-03-16", false},
		{"7d", "2026-06-12", false},
		{"2w", "2026-06-05", false},
		{"24h", "2026-06-18", false},
		{"banana", "", true},
		{"", "", true},
		{"-5d", "", true}, // a negative window would point into the future
		{"-1w", "", true},
	}
	for _, tc := range tests {
		got, err := ParseSince(now, tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseSince(%q): want error, got %v", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSince(%q): %v", tc.in, err)
			continue
		}
		if got.Format(ISO) != tc.want {
			t.Errorf("ParseSince(%q) = %s, want %s", tc.in, got.Format(ISO), tc.want)
		}
	}
}

// TestOnOrAfterNonUTC pins the timezone fix: in a zone west of UTC, an entry
// stamped for the cutoff's calendar day must still match. Parsing the stored
// date as UTC (while the cutoff is local) used to exclude it, breaking --since
// for every US user. UTC-pinned tests could not catch this.
func TestOnOrAfterNonUTC(t *testing.T) {
	est := time.FixedZone("EST", -5*3600)
	cutoff := time.Date(2026, 3, 18, 9, 0, 0, 0, est) // 9am local "today"
	cases := []struct {
		date string
		want bool
	}{
		{"2026-03-18", true},  // same calendar day as the cutoff
		{"2026-03-19", true},  // after
		{"2026-03-17", false}, // before
	}
	for _, c := range cases {
		if got := OnOrAfter(c.date, cutoff); got != c.want {
			t.Errorf("OnOrAfter(%q, EST cutoff) = %v, want %v", c.date, got, c.want)
		}
	}
}

// TestInWeekNonUTC mirrors TestOnOrAfterNonUTC for the weekly window.
func TestInWeekNonUTC(t *testing.T) {
	est := time.FixedZone("EST", -5*3600)
	ws := WeekStart(time.Date(2026, 3, 18, 9, 0, 0, 0, est)) // Monday 2026-03-16 EST
	if !InWeek("2026-03-16", ws) {
		t.Error("InWeek(week-start day, EST) = false, want true")
	}
	if !InWeek("2026-03-22", ws) {
		t.Error("InWeek(last day of week, EST) = false, want true")
	}
	if InWeek("2026-03-15", ws) {
		t.Error("InWeek(day before week, EST) = true, want false")
	}
}

func TestWeekStartIsMonday(t *testing.T) {
	// 2026-06-19 is a Friday; its week starts Monday 2026-06-15.
	got := WeekStart(mustParse(t, "2026-06-19"))
	if got.Format(ISO) != "2026-06-15" {
		t.Errorf("WeekStart = %s, want 2026-06-15", got.Format(ISO))
	}
	if got.Weekday() != time.Monday {
		t.Errorf("WeekStart weekday = %s, want Monday", got.Weekday())
	}
	// A Monday maps to itself.
	mon := WeekStart(mustParse(t, "2026-03-16"))
	if mon.Format(ISO) != "2026-03-16" {
		t.Errorf("WeekStart(Monday) = %s, want 2026-03-16", mon.Format(ISO))
	}
}

func TestInWeek(t *testing.T) {
	ws := mustParse(t, "2026-03-16") // Monday
	in := []string{"2026-03-16", "2026-03-19", "2026-03-22"}
	out := []string{"2026-03-15", "2026-03-23"}
	for _, d := range in {
		if !InWeek(d, ws) {
			t.Errorf("InWeek(%s) = false, want true", d)
		}
	}
	for _, d := range out {
		if InWeek(d, ws) {
			t.Errorf("InWeek(%s) = true, want false", d)
		}
	}
}

func TestParseWeek(t *testing.T) {
	now := mustParse(t, "2026-06-19")
	got, err := ParseWeek(now, "")
	if err != nil || got.Format(ISO) != "2026-06-15" {
		t.Errorf("ParseWeek(empty) = %s, %v", got.Format(ISO), err)
	}
	got, err = ParseWeek(now, "2026-03-19")
	if err != nil || got.Format(ISO) != "2026-03-16" {
		t.Errorf("ParseWeek(date) = %s, %v", got.Format(ISO), err)
	}
	if _, err := ParseWeek(now, "nope"); err == nil {
		t.Error("ParseWeek(bad): want error")
	}
}
