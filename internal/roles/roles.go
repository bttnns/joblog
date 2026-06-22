// Package roles imports and queries the deduped role index. It is pure: no file
// IO and no network. The CLI loads and saves roles.json via the store and calls
// these functions. Import ingests a jobhive JSON payload and computes the delta
// against the existing index; Filter and Find query it. See DESIGN.md sections
// "Roles" and "Scraper schema mapping".
package roles

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/bttnns/joblog/internal/dates"
	"github.com/bttnns/joblog/internal/model"
)

// LastImportRelPath is where the CLI persists the most recent import delta so
// `role changes` can show it, relative to the data dir. The roles package owns
// the on-disk layout of role data so the store stays a generic persistence layer.
const LastImportRelPath = "data/roles/last-import.json"

// ImportArchiveRelPath is where a raw import payload is archived for provenance,
// relative to the data dir.
func ImportArchiveRelPath(date, company string) string {
	return filepath.Join("data", "roles", "imports", date, company+".json")
}

// ImportResult is the delta from one import, JSON-serializable so the CLI can
// persist it for `roles changes`.
type ImportResult struct {
	Company string   `json:"company"`
	At      string   `json:"at"`      // ISO timestamp of the import
	New     []string `json:"new"`     // global_ids
	Changed []string `json:"changed"` // global_ids
	Gone    []string `json:"gone"`    // global_ids
}

// Counts returns the number of new, changed, and gone roles in the delta.
func (r ImportResult) Counts() (newN, changedN, goneN int) {
	return len(r.New), len(r.Changed), len(r.Gone)
}

// jobhiveRole mirrors the fields of jobhive's ~29-field scrape schema that we
// care about. Unknown fields are ignored by encoding/json for forward
// compatibility. posted_at and fetched_at may be epoch-millis integers (pandas)
// or RFC3339 strings or null, so they are decoded leniently and not stored.
type jobhiveRole struct {
	GlobalID      string   `json:"global_id,omitempty"`
	URL           string   `json:"url,omitempty"`
	Title         string   `json:"title,omitempty"`
	Company       string   `json:"company,omitempty"`
	ATSType       string   `json:"ats_type,omitempty"`
	ATSID         string   `json:"ats_id,omitempty"`
	Location      string   `json:"location,omitempty"`
	IsRemote      *bool    `json:"is_remote,omitempty"`
	SalarySummary string   `json:"salary_summary,omitempty"`
	SalaryMin     *float64 `json:"salary_min,omitempty"`
	SalaryMax     *float64 `json:"salary_max,omitempty"`
	Description   string   `json:"description,omitempty"`
	// posted_at and fetched_at may be epoch-millis numbers, RFC3339 strings, or
	// null. We do not store them, so RawMessage accepts any JSON form without a
	// decode error.
	PostedAt  json.RawMessage `json:"posted_at,omitempty"`
	FetchedAt json.RawMessage `json:"fetched_at,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

// salary derives Role.Salary, preferring the human-readable summary, then a
// formatted min/max range, else "".
func (j jobhiveRole) salary() string {
	if s := strings.TrimSpace(j.SalarySummary); s != "" {
		return s
	}
	switch {
	case j.SalaryMin != nil && j.SalaryMax != nil:
		return fmt.Sprintf("%s - %s", formatMoney(*j.SalaryMin), formatMoney(*j.SalaryMax))
	case j.SalaryMin != nil:
		return formatMoney(*j.SalaryMin) + "+"
	case j.SalaryMax != nil:
		return "up to " + formatMoney(*j.SalaryMax)
	}
	return ""
}

// formatMoney renders a salary figure without a trailing ".00" when it is a
// whole number.
func formatMoney(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("$%d", int64(v))
	}
	return fmt.Sprintf("$%.2f", v)
}

// remote derives Role.Remote: the explicit is_remote flag when non-null, else a
// best-effort inference from a "remote" mention in title or location.
func (j jobhiveRole) remote() bool {
	if j.IsRemote != nil {
		return *j.IsRemote
	}
	return mentionsRemote(j.Title) || mentionsRemote(j.Location)
}

func mentionsRemote(s string) bool {
	return strings.Contains(strings.ToLower(s), "remote")
}

// key returns the opaque index key. We prefer an explicit global_id; if jobhive
// does not provide one (the live scrape feed omits it), we compose the canonical
// {ats_type}:{ats_id} form, which is short and stable; failing that we fall back
// to the url so we still have an identity. The key is treated as opaque and is
// NEVER parsed once stored.
func (j jobhiveRole) key() string {
	if k := strings.TrimSpace(j.GlobalID); k != "" {
		return k
	}
	ats := strings.TrimSpace(j.ATSType)
	id := strings.TrimSpace(j.ATSID)
	if ats != "" && id != "" {
		return ats + ":" + id
	}
	return normalizeURL(j.URL)
}

// normalizeURL canonicalizes a URL used as a fallback identity so the same role
// re-scraped later keeps the same key: lowercased host, no query or fragment, no
// trailing slash. It returns the trimmed input unchanged when it does not parse.
// (net/url is parsing only; jl still makes no network calls.)
func normalizeURL(raw string) string {
	s := strings.TrimSpace(raw)
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return s
	}
	u.Host = strings.ToLower(u.Host)
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String()
}

// toRole builds a Role from the payload, stamping FirstSeen/LastSeen with iso.
// employerFallback is the --company argument, used when the payload omits a
// company name. companySlug is the canonical key the role is imported under; it
// scopes gone-marking and links the role to the company and any applications.
func (j jobhiveRole) toRole(employerFallback, companySlug, iso string) model.Role {
	employer := strings.TrimSpace(j.Company)
	if employer == "" {
		employer = employerFallback
	}
	return model.Role{
		GlobalID:    j.key(),
		Title:       j.Title,
		Employer:    employer,
		Company:     companySlug,
		Location:    j.Location,
		URL:         j.URL,
		Salary:      j.salary(),
		Description: j.Description,
		Remote:      j.remote(),
		FirstSeen:   iso,
		LastSeen:    iso,
		Status:      model.RoleOpen,
	}
}

// ImportOption tunes Import. The zero set of options is the default behavior.
type ImportOption func(*importConfig)

type importConfig struct {
	markGone bool
}

// SkipGoneMarking imports without retiring roles absent from the payload. Use it
// for a deliberately partial import (one page of a paginated scrape, a filtered
// export) so missing roles are not wrongly marked gone.
func SkipGoneMarking() ImportOption {
	return func(c *importConfig) { c.markGone = false }
}

// OpenCountForCompany counts the open roles currently attributed to companySlug,
// using the same canonical-slug identity Import stamps (with the legacy fallback
// to the slug of the employer for roles imported before the slug existed). The
// CLI uses it to gauge how destructive an import's gone-marking would be.
func OpenCountForCompany(existing []model.Role, companySlug string) int {
	if companySlug == "" {
		return 0
	}
	n := 0
	for _, r := range existing {
		if r.Status != model.RoleOpen {
			continue
		}
		slug := r.Company
		if slug == "" {
			slug = model.Slug(r.Employer)
		}
		if slug == companySlug {
			n++
		}
	}
	return n
}

// Import parses a jobhive JSON payload (a JSON array of role objects) for
// company, upserts into existing, marks roles previously seen for this company
// but absent from the payload as gone (unless SkipGoneMarking is passed), and
// returns the updated index plus the delta. now stamps first_seen/last_seen.
func Import(existing []model.Role, payload []byte, company string, now time.Time, opts ...ImportOption) ([]model.Role, ImportResult, error) {
	cfg := importConfig{markGone: true}
	for _, o := range opts {
		o(&cfg)
	}

	var raw []jobhiveRole
	if err := json.Unmarshal(payload, &raw); err != nil {
		return existing, ImportResult{}, fmt.Errorf("parse jobhive payload: %w", err)
	}

	iso := now.Format(dates.ISO)
	companySlug := model.Slug(company)
	result := ImportResult{
		Company: company,
		At:      now.Format(time.RFC3339),
	}

	// Work on a copy so Import never mutates the caller's slice (matching Filter's
	// contract); existing order is preserved.
	out := make([]model.Role, len(existing))
	copy(out, existing)

	// Index existing roles by key for upsert.
	index := make(map[string]int, len(out))
	for i := range out {
		index[out[i].GlobalID] = i
	}

	// Track every key present in this payload so we can scope gone-marking.
	present := make(map[string]bool, len(raw))

	for _, jr := range raw {
		key := jr.key()
		if key == "" {
			continue // no usable identity; skip rather than crash.
		}
		role := jr.toRole(company, companySlug, iso)
		if strings.TrimSpace(role.Employer) == "" {
			continue // reject: no employer identity (pass --company or a payload company)
		}
		present[key] = true

		i, ok := index[key]
		if !ok {
			out = append(out, role)
			index[key] = len(out) - 1
			result.New = append(result.New, key)
			continue
		}

		// Upsert: refresh fields, bump LastSeen, detect material changes.
		cur := out[i]
		incoming := role
		incoming.FirstSeen = cur.FirstSeen // preserve original first sighting.

		changed := cur.Title != incoming.Title ||
			cur.Location != incoming.Location ||
			cur.Salary != incoming.Salary ||
			cur.URL != incoming.URL ||
			cur.Description != incoming.Description

		incoming.Status = model.RoleOpen // present in payload => open (revives gone).
		out[i] = incoming
		if changed {
			result.Changed = append(result.Changed, key)
		}
	}

	// Gone-marking, scoped to THIS company by its canonical slug (NOT the
	// free-text employer, which can differ from the import label and silently
	// disable gone-marking): existing open roles imported under companySlug whose
	// key is absent from this payload become gone. With no company scope we mark
	// nothing, since we cannot tell which roles this payload was meant to cover.
	if cfg.markGone && companySlug != "" {
		for i := range out {
			r := out[i]
			if r.Status != model.RoleOpen {
				continue
			}
			if present[r.GlobalID] {
				continue
			}
			if r.Company != companySlug {
				continue
			}
			out[i].Status = model.RoleGone
			result.Gone = append(result.Gone, r.GlobalID)
		}
	}

	return out, result, nil
}

// defaultLaneKeywords is the in-code fallback used when no Lanes map is set on
// the Query. It mirrors the shipped lanes.yaml default so the filter works
// without a data dir.
var defaultLaneKeywords = map[string][]string{
	"reliability": {
		"site reliability", "sre", "platform engineer", "infrastructure engineer",
		"reliability engineer", "devops", "cloud engineer",
	},
	"devex": {
		"developer advocate", "developer relations", "devrel", "field cto",
		"devex", "developer experience", "technical evangelist",
	},
	"fixer": {
		"solutions architect", "solutions engineer", "field engineer",
		"technical account", "customer success engineer",
	},
}

// Query filters the index. Zero-value fields mean "no constraint".
type Query struct {
	Since    time.Time           // keep roles active on/after this; zero = no bound
	New      bool                // first seen within the Since window
	Changed  bool                // last changed within the Since window
	Gone     bool                // status == gone
	Employer string              // case-insensitive substring match on Employer
	Title    string              // case-insensitive substring match on Title
	Search   string              // case-insensitive substring match on Title+Employer+Description
	Remote   *bool               // nil = any
	Lane     string              // lane name matched against Title; keys must exist in Lanes
	Lanes    map[string][]string // keyword map loaded from lanes.yaml; nil uses the built-in default
}

// Filter returns the roles matching q. Filters combine with AND. The returned
// slice is a fresh slice; the input is not modified.
func Filter(roles []model.Role, q Query) []model.Role {
	hasSince := !q.Since.IsZero()
	out := make([]model.Role, 0, len(roles))

	for _, r := range roles {
		// Status gate: --gone is opt-in; otherwise only open roles.
		if q.Gone {
			if r.Status != model.RoleGone {
				continue
			}
		} else if r.Status != model.RoleOpen {
			continue
		}

		// --new: first seen on/after Since. With no Since bound it is a no-op.
		if q.New && hasSince {
			if !dates.OnOrAfter(r.FirstSeen, q.Since) {
				continue
			}
		}

		// --changed: last seen on/after Since and the role has actually changed
		// since first sighting.
		if q.Changed {
			if hasSince && !dates.OnOrAfter(r.LastSeen, q.Since) {
				continue
			}
			if r.LastSeen == r.FirstSeen {
				continue
			}
		}

		// --since alone (no --new/--changed): last seen on/after Since.
		if hasSince && !q.New && !q.Changed {
			if !dates.OnOrAfter(r.LastSeen, q.Since) {
				continue
			}
		}

		if q.Employer != "" && !containsFold(r.Employer, q.Employer) {
			continue
		}
		if q.Title != "" && !containsFold(r.Title, q.Title) {
			continue
		}
		if q.Search != "" && !containsFold(r.Title+" "+r.Employer+" "+r.Description, q.Search) {
			continue
		}
		if q.Remote != nil && r.Remote != *q.Remote {
			continue
		}
		if q.Lane != "" && !matchesLane(r.Title, q.Lane, q.Lanes) {
			continue
		}

		out = append(out, r)
	}
	return out
}

func matchesLane(title, lane string, lanes map[string][]string) bool {
	if lanes == nil {
		lanes = defaultLaneKeywords
	}
	kws := lanes[strings.ToLower(lane)]
	t := strings.ToLower(title)
	for _, kw := range kws {
		if strings.Contains(t, kw) {
			return true
		}
	}
	return false
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// Find resolves an id that is either a full global_id or an unambiguous prefix.
// A full-id match wins outright. Otherwise a prefix match succeeds only when it
// is unique.
func Find(roles []model.Role, idOrPrefix string) (model.Role, bool) {
	if idOrPrefix == "" {
		return model.Role{}, false
	}
	var match model.Role
	found := 0
	for _, r := range roles {
		if r.GlobalID == idOrPrefix {
			return r, true // exact match is unambiguous.
		}
		if strings.HasPrefix(r.GlobalID, idOrPrefix) {
			match = r
			found++
		}
	}
	if found == 1 {
		return match, true
	}
	return model.Role{}, false
}
