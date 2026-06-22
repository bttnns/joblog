package cli

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/bttnns/joblog/internal/catalog"
)

// atsMatch is the result of parsing a careers or posting URL: the ATS the
// scraper needs and the company slug. An empty ATS means the host is not a known
// multi-tenant board, so the company stays custom.
type atsMatch struct {
	ATS  string
	Slug string
}

// hostRule maps a multi-tenant ATS host to its scraper ATS name and where the
// slug lives. Only multi-tenant hosts are listed: hosts where one domain serves
// many companies and the company is named by the host or the first path segment.
// Single-tenant company domains (tesla.com, apple.com) are deliberately absent;
// their scrapers are unreliable in jobhive v0.1.0, so those stay custom.
type hostRule struct {
	host    string // matched as an exact host or, with a leading ".", a suffix
	ats     string
	subSlug bool // slug is the leftmost host label (e.g. <slug>.recruitee.com)
}

// atsHostRules is the table of confirmed multi-tenant boards. For a path-slug
// host the slug is the first path segment; for a subdomain-slug host (subSlug)
// the slug is the leftmost label of the host. Keep this list deterministic and
// confirmed: an unrecognized host falls through to custom.
var atsHostRules = []hostRule{
	{host: "boards.greenhouse.io", ats: "greenhouse"},
	{host: "job-boards.greenhouse.io", ats: "greenhouse"},
	{host: "boards.eu.greenhouse.io", ats: "greenhouse"},
	{host: "jobs.ashbyhq.com", ats: "ashby"},
	{host: "jobs.lever.co", ats: "lever"},
	{host: "apply.workable.com", ats: "workable"},
	{host: "jobs.workable.com", ats: "workable"},
	{host: "jobs.smartrecruiters.com", ats: "smartrecruiters"},
	{host: "careers.smartrecruiters.com", ats: "smartrecruiters"},
	{host: ".recruitee.com", ats: "recruitee", subSlug: true},
	{host: ".teamtailor.com", ats: "teamtailor", subSlug: true},
}

// parseATSURL parses a careers board root or a specific posting URL into an ATS
// and slug. It accepts a board root or a deep posting link: only the host and
// first path segment matter. An unrecognized host returns a zero match (empty
// ATS), which the caller treats as custom. An input that is not a URL at all
// (no scheme, no dot) is rejected so a bare flag value is not mistaken for one.
func parseATSURL(raw string) (atsMatch, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return atsMatch{}, fmt.Errorf("empty URL")
	}
	// Tolerate a scheme-less URL (boards.greenhouse.io/acme) by assuming https.
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return atsMatch{}, fmt.Errorf("parse URL %q: %w", raw, err)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || !strings.Contains(host, ".") {
		return atsMatch{}, fmt.Errorf("not a URL: %q", raw)
	}
	host = strings.TrimPrefix(host, "www.")

	segs := pathSegments(u.Path)
	for _, r := range atsHostRules {
		if r.subSlug {
			if suffix := strings.TrimPrefix(r.host, "."); strings.HasSuffix(host, "."+suffix) {
				label := strings.TrimSuffix(host, "."+suffix)
				label = label[strings.LastIndex(label, ".")+1:]
				if label == "" {
					break
				}
				return atsMatch{ATS: r.ats, Slug: label}, nil
			}
			continue
		}
		if host == r.host {
			if len(segs) == 0 {
				return atsMatch{}, fmt.Errorf("no company slug in %q (expected %s/<slug>)", raw, host)
			}
			return atsMatch{ATS: r.ats, Slug: segs[0]}, nil
		}
	}
	// Unrecognized host: stays custom.
	return atsMatch{}, nil
}

// careersURLFor reconstructs a careers URL from an ATS and slug, the reverse of
// parseATSURL. The host knowledge lives in one place (the catalog package, which
// drops the redundant url column from its snapshot and rebuilds it the same way);
// this is a thin alias so company.go reads naturally. An ATS the table does not
// know returns "".
func careersURLFor(ats, slug string) string { return catalog.URL(ats, slug) }

// pathSegments splits a URL path into its non-empty segments.
func pathSegments(p string) []string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// companyNameFromSlug derives a display name from a slug when --name is absent:
// hyphens and underscores become spaces and each word is title-cased, so
// "acme-corp" becomes "Acme Corp". It is a convenience default; the user can
// always pass --name to override.
func companyNameFromSlug(slug string) string {
	parts := strings.FieldsFunc(slug, func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(p)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}
