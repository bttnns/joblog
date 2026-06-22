---
name: joblog
description: Drive a token-efficient, AI-assisted job search using the jl CLI plus the jobhive scraper. Use when the user wants to find or rank open roles, decide whether to apply to a posting, build or update their job-search profile, research a company before an interview, keep their application pipeline honest, or stay compliant with their state's unemployment work-search requirements.
---

# joblog

`jl` is a small local CLI (the command is `jl`; the project is "joblog") that
remembers your job search: it tracks the companies you are interested in, keeps a
deduped index of roles you have seen (so it can surface what is new or changed),
tracks every work-search activity, stores your resume as cheap-to-read text, and
renders your state's weekly unemployment work-search report. It does **not**
scrape; that is delegated to a scraper you run (jobhive by default; JobSpy or a
browser "Copy as cURL" export work too).

This skill is the reasoning layer on top of that deterministic plumbing. The
primary job is **discovery**: turn "I saw this company" into tracked companies and
ranked roles you should actually look at. `jl` is built on a few co-equal
principles, not one overriding goal: **deterministic** (code renders, the agent
does not format compliance), **auditable** (plain local files you can inspect and
diff), **token-efficient** (read `resume/resume.txt` and `profile.md` once; look at role
**deltas**, not the whole market; trust the CLI to filter, diff, and format),
**minimal-dependency**, and **agent-portable** (the pipe is the contract). A
scraper does the fetching and pagination for zero agent tokens.

## What this skill does

It lets an agent operate the whole pipeline on the user's behalf, tuned to
their stated profile and next-role wants:

- Track a company the moment the user notices it: look it up, confirm its jobs
  are fetchable, add it, and import its roles (`track-company`).
- Suggest new companies to track from the pattern the user already gravitates to,
  searching a built-in offline catalog with `jl company search` (`suggest-companies`).
- Build the user's profile from their resume.
- Discover and rank new/changed roles against what they want next.
- Triage a single posting: should they apply, and what to lead with.
- Research a company before an interview.
- Keep the application pipeline honest (stale apps, status reconciliation).
- Keep them compliant with their state's weekly work-search rules.

## Prerequisites (check these first)

Before running the loop, verify setup. If something is missing, set it up or
tell the user how. The fastest check is `jl status`: it prints a checklist of
every setup step (resume, state, profile, companies, roles, weekly compliance)
marked done or todo, each todo showing the next command to run.

1. **jl installed**: `jl version`. If absent, point the user at the prebuilt
   binary for their platform on the
   [latest release](https://github.com/bttnns/joblog/releases/latest)
   (`jl_<version>_{darwin,linux}_{amd64,arm64}.tar.gz`): download, `tar -xzf`,
   and move `jl` onto their `PATH`. From-source fallback (needs Go 1.25+):
   `go install github.com/bttnns/joblog/cmd/jl@latest`.
2. **A scraper installed** (the default producer): jobhive, `jobhive --help`. If
   absent: `uv tool install jobhive-py` (source: `github.com/kalil0321/ats-scrapers`).
   JobSpy (`github.com/speedyapply/JobSpy`, board aggregators) and a browser
   "Copy as cURL" export are equally valid producers; jl imports any ATS job JSON.
3. **Data dir scaffolded**: `jl init` (idempotent). Resolves
   `$JOBLOG_HOME` then `$XDG_DATA_HOME/joblog` then `./private`.
4. **Resume stored**: `jl resume set <file.pdf|md|json>` writes a plaintext
   `resume/resume.txt` you can read cheaply. `jl resume` (or `jl resume ls`) lists the
   base resume and any per-role tailored variants.
5. **State set**: `jl config set state <code>` (e.g. `tx`). Set the weekly
   minimum if the state varies by county: `jl config set min N`.
6. **Profile built**: `profile.md` (who you are plus what you want next) and
   `accomplishments.md` exist at the data-dir root and are filled. If not, run the
   `build-profile` prompt mode.

## The core loop

The recurring request is "I'm looking at <company>, here's a URL, go figure it
out." For a brand-new company this is the `track-company` mode; the steps are:

1. **Identify ATS + slug** from the careers URL (e.g. a `boards.greenhouse.io/acme`
   URL means ATS `greenhouse`, slug `acme`). `jl company add <url>` parses both
   from a recognized multi-tenant board URL for you. If you already track the
   company, `jl company ls` has it.
2. **Scrape and import** (a scraper fetches, jl ingests the JSON):
   ```
   jobhive scrape <ats> <slug> --format json | jl role import - --company <name>
   ```
   `--format json` is **required**: jobhive defaults to a non-pipeable text table.
   `roles import` upserts into the index by `global_id`, marks roles absent from
   this import as `gone`, and prints a terse delta (N new / M changed / K gone).
   It expects a **full** scrape of the company: if you are importing only part of
   one (a single page, a filtered export), pass `--no-gone` so missing roles are
   not retired. An import that would retire more than half of a company's open
   roles is refused (it usually means a truncated scrape); re-run with `--force`
   only if the shrink is real.
3. **Review what changed**:
   ```
   jl role ls --since 7d --new
   ```
   This is pre-filtered and omits the big `description` field to save tokens;
   pull full detail for a single role with `jl role show <id>`.
4. **Rank against the profile** (`suggest-roles` mode): read `profile.md`, score
   fit, give reasons, surface a short list, skip the noise.
5. **Track an application** (only after the human decides):
   ```
   jl log add --from-role <id>
   ```
   `--from-role` pre-fills employer/title/url from the imported role, and links
   the role's tailored resume if you stored one with
   `jl resume add --role <id> <file>` (override with `--resume <id>`). Advance the
   application's status later with
   `jl log update <id> --status screen|onsite|offer|rejected`.
6. **Stay compliant**:
   ```
   jl report          # this week's work-search report
   jl report --check  # compliance exit code only
   ```

## Pipe contract (the lowest common denominator)

Every jl command is non-interactive, takes `--json` where useful, writes
errors to stderr with a nonzero exit, and reads import data from stdin (`-`). Any
producer that emits ATS job JSON works: jobhive (`--format json`), JobSpy, or a
browser "Copy as cURL" export. So the universal interface is a **pipe**:
`<producer> | jl <verb> -`. This sidesteps every per-tool difference. When in
doubt, use the pipe.

## Per-CLI install and invocation

The skill follows the Agent Skills open standard. Frontmatter is only `name`
and `description`; tool-specific keys are ignored elsewhere, so it stays portable.

- **Claude Code**: from a clone of the repo, symlink this folder so edits stay in
  sync: `ln -s "$PWD/skill" ~/.claude/skills/joblog`
  (the file must live at `~/.claude/skills/joblog/SKILL.md`). Or pipe a prompt headless:
  `jl profile build | claude -p "build my profile"`. (Caveat: a known bug
  can blank very large stdin; if that happens, write the data to a file and
  reference the path instead. For `role import` specifically, the import guard
  turns a truncated scrape into a loud refusal rather than a wiped index.)
- **pi** (`@mariozechner/pi-coding-agent`): natively loads the same SKILL.md
  from `~/.pi/agent/skills/joblog/`, invoked `/skill:joblog`. Headless:
  `jl profile build | pi -p "build my profile"` (use `-p`; bare `| pi` is
  interactive).
- **Codex**: reference this skill from the repo `AGENTS.md` (native SKILL.md
  auto-load is unconfirmed). Headless:
  `jl profile build | codex exec "build my profile" < /dev/null`. The
  `< /dev/null` avoids a hang when spawned in a non-TTY shell.

Exact agent flags vary by tool and version; the pipe is the contract.

## Session-gated board fallback

Some boards (e.g. ones behind a login or aggressive bot protection) jobhive
cannot reach for free. For these, ask the user to open the board in a browser,
right-click the roles XHR request in DevTools, and choose **Copy as cURL**. Run
that cURL command yourself and pipe its JSON output straight into jl:

```
<the pasted curl ...> | jl role import - --company <name>
```

The user is the session; you just relay the response into the index.

## Prompt modes

Load the matching file from `prompts/` for the task at hand:

- `prompts/track-company.md`: "I saw <company>" -> look it up, confirm jobs are
  fetchable, `jl company add`, and import its roles.
- `prompts/suggest-companies.md`: recommend new companies to track from the
  user's existing list + profile, surfaced offline with `jl company search`
  (a built-in catalog, no network); note which are active vs paused.
- `prompts/build-profile.md`: fill `profile.md` + `accomplishments.md` from `resume/resume.txt`.
- `prompts/discover.md`: scrape the company list, import, surface new roles.
- `prompts/suggest-roles.md`: rank imported roles against the profile.
- `prompts/triage-role.md`: given one role URL, verdict on fit + what to lead with.
- `prompts/research-company.md`: deep-dive to `companies/<name>/research.md` + fit notes.
- `prompts/weekly-compliance.md`: check compliance, draft the state report.

Per-company material lives at `companies/<slug>/` in the data dir (the
`company.md` stub plus research, plan, and tailored notes together). `jl company
add` scaffolds the folder; `jl company show <name>` lists what is in it.

## Safety

These are hard rules, and this section is the canonical statement of them (other
prompt modes point here rather than restating). The agent **prepares**; the human
**reviews and submits**. This is also enforced structurally: `jl` has no submit
or certify verb and makes no network calls, so it *cannot* act on the user's
behalf even if asked.

- **Never auto-submit a job application.** Draft, fill, and stage; the human
  clicks submit.
- **Never certify unemployment or file a weekly claim.** Draft the work-search
  log for the human to paste into the state portal themselves.
- **Always surface the state's source URL.** `jl report` and state setup
  print the official workforce page; relay it.
- **Never present compliance data as authoritative.** Requirements change and
  vary by county. Always include the "verify at <state URL>" disclaimer. What
  jl encodes is researched and useful, not legal advice.

## Why it is useful (use cases)

1. **Weekly compliance on autopilot.** "Am I compliant?" counts activities,
   flags if short, and drafts the state log to paste into the portal.
2. **What's new worth my time.** Scrape targets, rank new roles by fit, return
   a short list with reasons, skip the noise.
3. **Should I apply?** Paste a URL, get a fit verdict against the user's
   experience, next-role wants, and salary, plus which stories to lead with.
4. **Keep the pipeline honest.** Flag stale applications, reconcile statuses with
   `jl log update`, prep for an interview (company research + matching accomplishments).
5. **For anyone.** Install the skill, point it at your resume and your state,
   get the same pipeline. That is the public value proposition.
