package state

import "github.com/bttnns/joblog/internal/model"

// Entry Type values. Mirrors model.EntryTypes; named here so the per-state
// qualify rules read clearly.
const (
	typeApplied         = "applied"
	typeNetworking      = "networking"
	typePhoneInterview  = "phone-interview"
	typeOnlineInterview = "online-interview"
	typeInPersonIntv    = "in-person-interview"
	typeJobFair         = "job-fair"
	typeWorkforceOffice = "workforce-office"
	typeOther           = "other"
)

// isInterview reports whether t is one of the three interview types.
func isInterview(t string) bool {
	return t == typePhoneInterview || t == typeOnlineInterview || t == typeInPersonIntv
}

// isApplication reports whether the entry is a direct application to an
// employer (an "employer contact" in most state vocabularies). An interview is
// downstream of a contact and counts too.
func isApplication(e model.Entry) bool {
	return e.Type == typeApplied || isInterview(e.Type)
}

// defaultQualify is the common rule used by most states: any entry whose Type
// is a genuine job-search activity counts. "other" never counts on its own,
// and "workforce-office" counts only for states that say so (those override
// qualify). job-fair, networking, applications, and interviews all count.
func defaultQualify(e model.Entry) bool {
	switch e.Type {
	case typeOther, typeWorkforceOffice:
		return false
	default:
		return true
	}
}

// countQualifying applies a per-entry predicate across the week.
func countQualifying(week []model.Entry, qualify func(model.Entry) bool) int {
	n := 0
	for _, e := range week {
		if qualify(e) {
			n++
		}
	}
	return n
}

// countDistinct counts the distinct non-empty keys produced by keyOf across the
// qualifying entries. It is how states that require N *different* employers or
// activities on N *different* days avoid over-crediting repeated contacts.
func countDistinct(week []model.Entry, qualify func(model.Entry) bool, keyOf func(model.Entry) string) int {
	seen := map[string]struct{}{}
	for _, e := range week {
		if !qualify(e) {
			continue
		}
		if k := keyOf(e); k != "" {
			seen[k] = struct{}{}
		}
	}
	return len(seen)
}

// countDistinctEmployers counts the distinct companies contacted, keyed by the
// canonical company slug (so "Fastly" and "Fastly Inc." count once), not the
// free-text employer string.
func countDistinctEmployers(week []model.Entry, qualify func(model.Entry) bool) int {
	return countDistinct(week, qualify, companyKey)
}

// countDistinctDays counts the distinct calendar days on which a qualifying
// activity occurred.
func countDistinctDays(week []model.Entry, qualify func(model.Entry) bool) int {
	return countDistinct(week, qualify, func(e model.Entry) string { return e.Date })
}

// companyKey is the canonical company slug for an entry, falling back to the
// slug of the employer for legacy entries with no Company set.
func companyKey(e model.Entry) string {
	if e.Company != "" {
		return e.Company
	}
	return model.Slug(e.Employer)
}

// meets returns ok = (min == 0 || n >= min); a min of 0 means "reasonable /
// unspecified", which is always considered satisfied.
func meets(n, min int) bool {
	return min == 0 || n >= min
}

// checkDefault is the Check implementation shared by states that use the
// default qualify rule. States with compound or custom rules implement Check
// themselves.
func checkDefault(week []model.Entry, min int) (int, bool) {
	n := countQualifying(week, defaultQualify)
	return n, meets(n, min)
}
