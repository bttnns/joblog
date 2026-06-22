# AGENTS.md

Instructions for an AI agent working **in** this repository. To operate a job
search (not work on the code), the manual is `skill/SKILL.md`.

## What this repo is

The CLI is `jl` (the project is "joblog"). It is a small, local-first Go CLI that
tracks companies you are interested in, indexes roles, logs job-search activity,
and renders state unemployment work-search reports. See `DESIGN.md` for the full
spec.

**Division of labor:** `jl` makes no network calls and does not scrape. It imports
ATS job JSON from any producer, jobhive by default
(`github.com/kalil0321/ats-scrapers`, PyPI `jobhive-py`), JobSpy for board
aggregators, or a browser "Copy as cURL" export, then tracks, reports, and
analyzes.

## Build / test / run

```
go build ./...
go test ./...        # table-driven tests; golden-file tests for each State.Render()
go run ./cmd/jl --help
```

Release builds stamp the version via ldflags (it defaults to `dev`):

```
go build -ldflags "-X github.com/bttnns/joblog/internal/cli.Version=$(git describe --tags)" ./cmd/jl
```

The golden-file tests cover compliance rendering (especially the TX BN900E), which
must be correct. When you change a `State.Render()`, update the corresponding
golden fixture and confirm the diff is intended before committing.

## Minimal-deps rule

`jl` deliberately uses just two third-party dependencies (cobra and
`dslipak/pdf`); everything else is stdlib. **Before adding any dependency**, make
the case for it in the PR: prefer stdlib (`text/tabwriter`, `crypto/rand`,
`encoding/json`, a hand-rolled duration parser), then a module already in
`go.mod`, and only then something new (vetted for license and maintenance).

## Install the skill and the scraper

- **Skill:** symlink the folder so edits stay in sync, `ln -s "$PWD/skill"
  ~/.claude/skills/joblog` for Claude Code (so `SKILL.md` lands at
  `~/.claude/skills/joblog/SKILL.md`), or `~/.pi/agent/skills/joblog/` for pi, or
  reference it from this `AGENTS.md` for Codex. The pipe contract
  (`... | <agent> -p "<prompt>"`) works across all three.
- **Scraper:** `uv tool install jobhive-py`. The jobhive source is at
  `github.com/kalil0321/ats-scrapers`.

## Public-safe rule

This repo goes public. Treat it as public from the start.

- **Never commit anything under `private/`.** It is gitignored; keep it that way.
  Per-company material lives in `private/companies/<name>/` and must never be
  tracked.
- **Keep every example synthetic.** Use placeholders like `acme-corp` and
  `example.com` in README, DESIGN, code comments, and fixtures. Never paste real
  company names, target lists, or personal data (names, phones, emails) into
  tracked files.
