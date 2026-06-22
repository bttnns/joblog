package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/bttnns/joblog/internal/dates"
	"github.com/bttnns/joblog/internal/model"
	"github.com/bttnns/joblog/internal/state"
	"github.com/bttnns/joblog/internal/store"
	"github.com/spf13/cobra"
)

func init() { addCommand(newStatusCmd) }

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show where you are in the setup and use flow, with the next step",
		Long: "Print a compact checklist of your setup: resume, state, profile, companies,\n" +
			"roles, and this week's work-search compliance. Each line is marked done or\n" +
			"todo, and a todo line shows the next command to run. Read-only.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd)
		},
	}
}

// statusCheck is one line of the status map: a named check, whether it is
// satisfied, a human detail, and the command to run next when it is not.
type statusCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Next   string `json:"next,omitempty"`
}

// runStatus renders the status map. It is read-only (openStore, never a write)
// so a bare `jl` is side-effect free; both the status command and the root
// command's RunE call this single implementation.
func runStatus(cmd *cobra.Command) error {
	s, err := openStore(cmd)
	if err != nil {
		return err
	}
	cfg, err := s.LoadConfig()
	if err != nil {
		return err
	}
	checks, err := buildStatusChecks(s, cfg)
	if err != nil {
		return err
	}

	if wantJSON(cmd) {
		return emitJSON(checks)
	}
	tw := newTabWriter()
	for _, c := range checks {
		mark := "todo"
		if c.OK {
			mark = "ok"
		}
		next := c.Next
		if c.OK {
			next = ""
		}
		fmt.Fprintf(tw, "[%s]\t%s\t%s\t%s\n", mark, c.Name, c.Detail, next)
	}
	tw.Flush()
	return nil
}

// buildStatusChecks computes the status lines in display order. It mirrors how
// report computes weekly compliance (state.Get, entriesInWeek for the current
// week, p.Check with the cfg.Min override) so the two never disagree.
//
// The Next hints are literal command strings; keep them in sync when a command
// is renamed (e.g. the resume/profile verbs), since nothing validates them at
// compile time.
func buildStatusChecks(s *store.Store, cfg store.Config) ([]statusCheck, error) {
	var checks []statusCheck

	// resume: stored if the config points at one, or resume/resume.txt exists.
	resumeOK := cfg.ResumePath != "" || fileExists(s.Path("resume", "resume.txt"))
	resumeDetail := "no resume stored"
	if resumeOK {
		resumeDetail = "stored"
	}
	checks = append(checks, statusCheck{
		Name:   "resume",
		OK:     resumeOK,
		Detail: resumeDetail,
		Next:   "jl resume set <file>",
	})

	// state: configured state code drives the weekly report.
	stateOK := cfg.State != ""
	stateDetail := "not set"
	if stateOK {
		stateDetail = cfg.State
	}
	checks = append(checks, statusCheck{
		Name:   "state",
		OK:     stateOK,
		Detail: stateDetail,
		Next:   "jl config set state <code>",
	})

	// profile: profile.md exists and looks filled in (not the all-TODO template).
	profileOK := profileFilled(s.Path("profile.md"))
	profileDetail := "not built"
	if profileOK {
		profileDetail = "built"
	}
	checks = append(checks, statusCheck{
		Name:   "profile",
		OK:     profileOK,
		Detail: profileDetail,
		Next:   "jl profile edit",
	})

	// companies: count tracked.
	companies, err := s.LoadCompanies()
	if err != nil {
		return nil, err
	}
	checks = append(checks, statusCheck{
		Name:   "companies",
		OK:     len(companies) > 0,
		Detail: fmt.Sprintf("%d tracked", len(companies)),
		Next:   "jl company add ...",
	})

	// roles: count of open roles in the index.
	roleList, err := s.LoadRoles()
	if err != nil {
		return nil, err
	}
	open := 0
	for _, r := range roleList {
		if r.Status == model.RoleOpen {
			open++
		}
	}
	checks = append(checks, statusCheck{
		Name:   "roles",
		OK:     open > 0,
		Detail: fmt.Sprintf("%d open", open),
		Next:   "jl fetch",
	})

	// this week: compliance vs the state minimum, computed exactly as report does.
	// Without a state there is nothing to measure, so the check is blocked on it.
	if cfg.State == "" {
		checks = append(checks, statusCheck{
			Name:   "this week",
			OK:     false,
			Detail: "blocked: set a state first",
			Next:   "jl config set state <code>",
		})
		return checks, nil
	}
	p, ok := state.Get(cfg.State)
	if !ok {
		checks = append(checks, statusCheck{
			Name:   "this week",
			OK:     false,
			Detail: fmt.Sprintf("unknown state %q", cfg.State),
			Next:   "jl config set state <code>",
		})
		return checks, nil
	}
	ws, err := dates.ParseWeek(nowFunc(), "")
	if err != nil {
		return nil, err
	}
	weekEntries, err := entriesInWeek(s, ws)
	if err != nil {
		return nil, err
	}
	min := p.MinDefault()
	if cfg.Min > 0 {
		min = cfg.Min
	}
	n, compliant := p.Check(weekEntries, min)
	checks = append(checks, statusCheck{
		Name:   "this week",
		OK:     compliant,
		Detail: fmt.Sprintf("%d/%d activities", n, min),
		Next:   "jl report",
	})
	return checks, nil
}

// profileFilled reports whether profile.md exists, has non-trivial content, and
// is not still the all-TODO scaffold. The heuristic is pragmatic: a profile a
// human has actually written has prose lines beyond the template placeholders.
func profileFilled(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// Count content lines that are neither comments, blank, headings, nor an
	// untouched "TODO" placeholder. A freshly scaffolded profile.md has none. The
	// template's guidance lives in multi-line HTML comments, so track comment
	// state across lines rather than only matching a leading "<!--".
	real := 0
	inComment := false
	for _, line := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(line)
		if inComment {
			if strings.Contains(t, "-->") {
				inComment = false
			}
			continue
		}
		switch {
		case t == "":
			continue
		case strings.HasPrefix(t, "#"):
			continue
		case strings.HasPrefix(t, "<!--"):
			if !strings.Contains(t, "-->") {
				inComment = true
			}
			continue
		}
		// Strip a leading list bullet, then treat a remaining "TODO..." as empty.
		t = strings.TrimSpace(strings.TrimPrefix(t, "- "))
		if t == "" || strings.HasPrefix(t, "TODO") {
			continue
		}
		// A "key: TODO" line (e.g. "Floor: TODO") is still a placeholder.
		if i := strings.Index(t, ":"); i >= 0 && strings.HasPrefix(strings.TrimSpace(t[i+1:]), "TODO") {
			continue
		}
		real++
	}
	return real > 0
}

// fileExists reports whether path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
