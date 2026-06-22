// Package state holds the per-state unemployment compliance profiles joblog
// tracks the work-search log against. A state is a compliance profile: how many
// activities you owe per week, which activity types count, the official form,
// how long to keep records, and the agency's render format. One superset Entry
// schema (see internal/model) stores everything; each profile is a small struct
// that encodes the rules plus the agency's weekly format.
//
// All values are researched from primary state-workforce sources (June 2026).
// Per-state citations and volatile-value flags live in each state's file. The
// data is genuinely useful but never authoritative: every Render ends with a
// disclaimer pointing to the official SourceURL.
package state

import (
	"sort"

	"github.com/bttnns/joblog/internal/model"
)

// Profile is one state's compliance rules plus its weekly render format. The
// CLI resolves the effective weekly minimum (config override, else MinDefault)
// and passes it to Check.
type Profile interface {
	Code() string                                       // "tx","ca",...
	Name() string                                       // "Texas"
	MinDefault() int                                    // default required activities/week; 0 = reasonable/unspecified
	Submit() bool                                       // true = submit weekly; false = keep + produce on audit
	FormName() string                                   // official form id
	Retention() string                                  // how long to keep
	SourceURL() string                                  // official state workforce page
	Check(week []model.Entry, min int) (n int, ok bool) // count qualifying activities; ok = meets min
	Render(week []model.Entry) string                   // the agency's weekly form/format text
}

// registry holds every implemented profile keyed by lowercase code. Each
// state's file registers itself in its init function.
var registry = map[string]Profile{}

// register adds a profile to the registry. It panics on a duplicate code so a
// programming mistake surfaces at startup rather than silently shadowing.
func register(p Profile) {
	if _, dup := registry[p.Code()]; dup {
		panic("state: duplicate profile code " + p.Code())
	}
	registry[p.Code()] = p
}

// Get looks up a profile by its lowercase code.
func Get(code string) (Profile, bool) {
	p, ok := registry[code]
	return p, ok
}

// All returns every profile, sorted by Code.
func All() []Profile {
	out := make([]Profile, 0, len(registry))
	for _, p := range registry {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code() < out[j].Code() })
	return out
}

// Codes returns every profile code, sorted, for help text.
func Codes() []string {
	out := make([]string, 0, len(registry))
	for code := range registry {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}
