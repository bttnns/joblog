package worklog

import (
	"testing"
	"time"

	"github.com/bttnns/joblog/internal/dates"
	"github.com/bttnns/joblog/internal/model"
)

func sample() []model.Entry {
	return []model.Entry{
		{ID: "1", Date: "2026-06-15", Type: "applied", Employer: "Acme Corp", Status: "applied"},
		{ID: "2", Date: "2026-06-16", Type: "networking", Employer: "Globex", Status: "awaiting"},
		{ID: "3", Date: "2026-06-22", Type: "applied", Employer: "Acme Robotics", Status: "applied"},
	}
}

func ids(es []model.Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.ID
	}
	return out
}

func eq(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestFilter(t *testing.T) {
	now := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	wk, _ := dates.ParseWeek(now, "") // week of 2026-06-15 (Mon) .. 2026-06-21
	since := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		q    Query
		want []string
	}{
		{"no constraint", Query{}, []string{"1", "2", "3"}},
		{"status", Query{Status: "applied"}, []string{"1", "3"}},
		{"type", Query{Type: "networking"}, []string{"2"}},
		{"employer substring fold", Query{Employer: "acme"}, []string{"1", "3"}},
		{"week", Query{Week: wk}, []string{"1", "2"}},
		{"since", Query{Since: since}, []string{"3"}},
		{"combined", Query{Status: "applied", Week: wk}, []string{"1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ids(Filter(sample(), tt.q)); !eq(got, tt.want...) {
				t.Errorf("Filter() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFilterDoesNotMutate verifies Filter leaves the caller's slice untouched.
func TestFilterDoesNotMutate(t *testing.T) {
	in := sample()
	snapshot := in[0]
	_ = Filter(in, Query{Status: "applied"})
	if in[0] != snapshot {
		t.Errorf("Filter mutated its input")
	}
}
