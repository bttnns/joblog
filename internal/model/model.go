// Package model defines the only two record types joblog stores: an Entry in
// the work-search log and a Role in the deduped roles index. Keeping the data
// model this small is deliberate (see DESIGN.md).
package model

import (
	"strings"
	"unicode"
)

// Entry is one item in the work-search log. Applying, networking, and
// interviewing are all the same thing: an entry whose Status moves over time.
type Entry struct {
	ID       string `json:"id"`
	Date     string `json:"date"` // YYYY-MM-DD
	Type     string `json:"type"`
	Employer string `json:"employer"`
	Company  string `json:"company,omitempty"` // canonical company slug; links to a company and its roles
	Title    string `json:"title"`
	JobType  string `json:"job_type"` // type of work sought (required by some state forms)
	URL      string `json:"url"`
	Method   string `json:"method"`
	Status   string `json:"status"`
	Contact  string `json:"contact"`
	Notes    string `json:"notes"`
	Resume   string `json:"resume,omitempty" yaml:"resume,omitempty"` // role id or relative path of the tailored resume used
}

// Role is one position in the roles index. The index is deduped by GlobalID and
// tracks when a role was first and last seen so changes surface over time.
type Role struct {
	GlobalID    string `json:"global_id"`
	Title       string `json:"title"`
	Employer    string `json:"employer"`          // human display name from the scraper payload
	Company     string `json:"company,omitempty"` // canonical company slug, set from the import label
	Location    string `json:"location"`
	URL         string `json:"url"`
	Salary      string `json:"salary"`
	Description string `json:"description,omitempty"`
	Remote      bool   `json:"remote"`
	FirstSeen   string `json:"first_seen"`
	LastSeen    string `json:"last_seen"`
	Status      string `json:"status"` // open | gone
}

// Slug normalizes a company name into a canonical key used to link an Entry, the
// roles scraped from a company, and that company's entry in the company list.
// It lowercases, collapses every run of non-alphanumeric characters to a single
// hyphen, and trims leading/trailing hyphens, so "Fastly", "fastly", and
// "Fastly!" all slug to "fastly". It is the one place company identity is
// derived; callers set it once at write-time rather than re-deriving on read.
func Slug(name string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if !lastHyphen && b.Len() > 0 {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// Controlled vocabularies. These are the allowed values for the matching Entry
// fields; they double as the help text and validation source.
var (
	EntryTypes = []string{
		"applied", "networking", "phone-interview", "online-interview",
		"in-person-interview", "job-fair", "workforce-office", "other",
	}
	Methods  = []string{"online-portal", "email", "phone", "in-person", "linkedin", "mail"}
	Statuses = []string{"applied", "screen", "onsite", "offer", "rejected", "awaiting", "no-reply"}
)

// Role status values.
const (
	RoleOpen = "open"
	RoleGone = "gone"
)

// Valid reports whether v is one of the allowed values.
func Valid(v string, allowed []string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
