# `joblog` design

This is the design spec for the `joblog` CLI. It is spec only: no implementation
beyond the small type and interface definitions shown here.

## Purpose

`joblog` is a small, local-first CLI for you and your AI agent to (1) log your
specific state's unemployment work-search requirements and (2) keep tabs on what
roles are open. It tracks your search history in a simple local JSON store,
reports against your state's weekly compliance rules, and imports/analyzes the
role JSON a scraper produces. jl itself makes no network calls; `jl fetch`
invokes an external scraper that does the HTTP, and jl ingests its JSON. See
"Division of labor" below.

A portable agent skill (`skill/SKILL.md`) sits on top and drives `joblog` plus
the recommended scraper. The CLI is the deterministic substrate; the skill is the
brain.

## Design philosophy: co-equal principles

There is no single overriding goal; `jl` balances a few co-equal principles:

- **Deterministic**: code renders compliance and lists; the model never formats
  the work-search report. Output is reproducible.
- **Auditable**: the store is a handful of plain local files (JSON/YAML) you can
  open, diff, and grep. No database, no opaque state.
- **Token-efficient**: the CLI remembers, diffs, filters, and formats so an
  AI agent spends tokens only on judgment, not on re-deriving the search each turn.
- **Minimal-dependency**: stdlib first; two third-party libraries today.
- **Agent-portable**: every command is non-interactive with `--json`; the pipe is
  the lowest-common-denominator contract across agent CLIs.

The token-efficiency principle in practice means: **push deterministic work into
the CLI so the LLM spends tokens only on judgment.** Concretely:

- The agent reads a compact `resume.txt` once, not a re-parsed PDF every turn.
- The agent sees role **deltas** (`role changes`, `role ls --new`) instead of
  re-scanning the whole market.
- The agent gets pre-filtered, structured `list` / `report` output, not raw JSON
  to sift.
- The agent reads a short `profile.md` instead of re-deriving what you want.
- The state report is **rendered by code**, not formatted by the model.
- The scraper does pagination and fetching, costing zero agent tokens.

The agent reasons; `joblog` remembers, filters, and formats. After token
efficiency the priorities are DRY, KISS, idiomatic Go, and minimal dependencies:
one store, one record type, thin commands. Every command emits compact,
pre-filtered output (`--json` for agents) precisely so an agent never has to load
or re-derive more than it needs.

## Stack

Just **two** third-party dependencies (versions verified 2026-06-19; see
`~/Dev/Docs/go-modules.md`):

- `github.com/spf13/cobra` **v1.10.2** (Apache-2.0): commands, flags, help. No
  `cobra-cli` generator; command files are hand-written. No viper (stdlib
  `os.ReadFile` plus `os.UserConfigDir` cover config).
- `github.com/dslipak/pdf` **v0.0.2** (BSD-3, pure Go, no cgo): resume PDF to
  text. We avoid `unidoc/unipdf` because it is AGPL/commercial.

Everything else is **stdlib**:

- `encoding/json`: the whole-file store.
- `text/tabwriter`: aligned `list` / `report` output, so no table library.
- `crypto/rand` plus `encoding/hex`: entry IDs.
- A roughly 15-line hand-rolled `--since 7d/2w` parser, because
  `time.ParseDuration` lacks days and weeks.

For YAML (`companies.yaml`, `config.yaml`) use `go.yaml.in/yaml/v3` **v3.0.4**, the
maintained YAML-org successor. Note: `gopkg.in/yaml.v3` was archived and
unmaintained as of April 2025; do not use it.

No `net/http`: `joblog` makes no network calls of its own; `jl fetch` shells out
to an external scraper that does the HTTP. Optional later additions: `rs/xid`
(sortable IDs), `fatih/color` (colored output).

## Package layout

```
cmd/jl/main.go
internal/
  model/      # Entry, Role, Slug: the only record types
  store/      # load/save log.json, roles.json, companies.yaml
  roles/      # import + query role JSON (from the scraper or a curl export); NO scraping
  state/      # state compliance profiles (tx, ca, ...)
  cli/        # cobra commands, one file per command
```

## Division of labor

Scraping is delegated to **jobhive** (`github.com/kalil0321/ats-scrapers`, PyPI
`jobhive-py`), used directly by you or your agent and documented in `AGENTS.md`.

`jl` itself makes **no** network calls. `jl fetch` shells out to an external
producer (a scraper) that does the HTTP, and `jl` ingests the ATS job JSON it
emits; you can also pipe that JSON in directly from any producer, jobhive by
default, JobSpy for board aggregators, or a browser curl export from a
session-gated board, then track, report, and analyze. This keeps `jl` a small
Go tool with zero scraping maintenance burden, and it keeps the scraping concern
(and its HTTP) entirely out of the binary. jobhive is the documented default, not
a hard dependency; the contract is "pipe in ATS job JSON". The human-safety
guarantee is structural and unchanged: `jl` has no submit verb and no certify
verb, so it cannot apply to a job or file an unemployment claim on your behalf.

## Data directory and scaffolding

One resolver picks the data directory by this precedence:

1. `$JOBLOG_HOME` if set.
2. `$XDG_DATA_HOME/joblog` (default `~/.local/share/joblog`).
3. `./private` if it exists in the working tree.

`--data-dir` overrides all of the above. Public users default to the XDG path so
their data is physically separate from any source tree and cannot be accidentally
committed. An existing in-repo `./private` keeps working as the fallback.

`jl init` scaffolds the whole tree wherever the resolver lands:

```
profile.md         # who you are and what you want next
accomplishments.md # master prose plus remixable STAR beats
resume/
companies/
data/
  log.json
  roles.json
  companies.yaml
config.yaml
README.md          # generated, documents the layout
```

It is idempotent (re-running never clobbers existing content) and produces the
same canonical structure that personal content is filed into. The `narrative/`
folder was flattened to the two root files above. Per-company material lives under
`companies/<slug>/`, scaffolded with a `company.md` stub by `jl company add`.

## Data model

Applying, networking, an interview: all the same thing, an entry in the
work-search log. "Application status" is just a field you update as the entry
moves. There is no apps-vs-activities split, no derived pipeline, no join tables.

```go
type Entry struct {
    ID, Date, Type, Employer, Company, Title, JobType, URL, Method, Status, Contact, Notes string
    // Company: canonical company slug, stamped at write-time, links an entry to a
    //          company and its roles (drives the company list's APPLIED column).
    // Type:    applied | networking | phone-interview | online-interview |
    //          in-person-interview | job-fair | workforce-office | other
    // JobType: "type of work sought": REQUIRED by TX (BN900E), IL, VA forms
    // Method:  online-portal | email | phone | in-person | linkedin | mail
    // Status:  applied | screen | onsite | offer | rejected | awaiting | no-reply
}
```

This superset of fields (date, activity type, employer, address, phone, contact,
method, job type, result/status, notes) was verified to cover every researched
state's required log fields.

```go
type Role struct {
    GlobalID, Title, Employer, Company, Location, URL, Salary, Description string
    Remote bool
    FirstSeen, LastSeen, Status string  // Status: open | gone
    // Company: canonical slug set from the import --company label; it scopes
    //          gone-marking and links the role to the company. Employer stays the
    //          human display name from the payload.
}
```

The company **slug** (`model.Slug`) is the one identity that links an `Entry`, the
`Role`s scraped from a company, and that company's row in `companies.yaml`. It is
set once at write-time, never re-derived by fuzzy string match on the employer.

The store is three whole-file read/write artifacts:

- `log.json`: the `Entry[]` work-search log.
- `roles.json`: the deduped `Role` index, queryable.
- `companies.yaml`: tracked companies for the scraper (renamed from `targets.yaml`,
  which is still read once for back-compat).

Raw scraper or curl payloads are optionally archived under `roles/imports/<date>/`
for provenance; `roles.json` is the queryable index.

**Schema version.** `log.json` and `roles.json` are written as a small versioned
envelope (`{"schema":N,"entries":[...]}`), so a future field change can be migrated
on load and a file written by a newer `jl` is refused rather than silently
misread. A legacy bare-array file (no envelope) is recognized as schema 0 and read
for back-compat.

**Concurrency.** Each mutating command takes an exclusive advisory lock on the
data directory for its load-modify-save cycle, so a human running `jl add` and an
agent running `jl role import` against the same data dir cannot lose each other's
write. The atomic write prevents a torn file; the lock prevents a lost update. The
lock is released on command exit (and by the OS on process exit, so a crash never
leaves it held).

## Command surface

The verb set is small and noun-grouped (`jl <noun> <verb>`). The work-search log
verbs keep hidden top-level aliases (`jl add`, `jl list`, `jl update`, `jl rm`)
for the daily path; `roles`, `target`, and `narrative` survive as hidden aliases
of their new names.

### Log (tracking)

Applications and activities share one record and one set of verbs.

- `jl log add` (alias `jl add`): add an entry. Flags: `--type` (default
  `applied`), `--employer`, `--company` (canonical slug; default derived from
  `--employer`), `--title`, `--job-type`, `--url`, `--method`, `--status`,
  `--contact`, `--notes`, `--from-role <id>` (pre-fill, and inherit the company
  slug, from an imported role).
- `jl log ls` (alias `jl list`, `jl log list`): list and filter entries. Flags:
  `--status`, `--employer`, `--type`, `--week`, `--since`. A bare `jl log` runs it.
- `jl log show <id>`: one entry's full detail.
- `jl log update <id>` (alias `jl update`): edit a field (e.g. advance `--status`).
- `jl log rm <id>` (alias `jl rm`): delete an entry.
- `jl report [--week] [--state] [--check]`: render the active state's weekly
  format from the log (top-level; it is the payoff). Defaults to the current week.
  `--check` returns a compliance exit code only (exit 2 when short, distinct from
  exit 1 for a tool failure).

### Role

`role` keeps a deduped index of every role seen and surfaces changes over time
(new, edited, gone). That diff is the highest-signal piece for an ongoing search,
and the gap a scraper does not fill: a scraper returns the current list with
within-fetch dedup but keeps no per-user history.

- `jl role import [file|-] [--company C]`: ingest ATS job JSON from any producer
  or a curl export. For example:

  ```
  jobhive scrape ashby acme-corp --format json | jl role import - --company acme-corp
  ```

  The import upserts into the index keyed by `global_id` (treated as an **opaque
  string**; see schema notes), updates `first_seen`/`last_seen`, and stamps the
  canonical company slug from `--company`. Roles imported under that slug and
  absent from the latest import are marked `gone` (scoped by the slug, not the
  free-text employer). It prints a terse delta and writes the index **atomically**.
  This is the only entry point for role data; there is no scraping in `jl`.
  A full scrape of a company is the assumed input: as a guard against a truncated
  or partial scrape (e.g. a harness that blanks large stdin), an import that would
  retire more than half of a company's open roles is **refused** unless `--force`.
  Use `--no-gone` for a deliberately partial import (one page, a filtered export)
  so missing roles are not retired at all.
- `jl role ls [--since 7d] [--new|--changed|--gone] [--employer --remote --title --search --lane]`
  (alias `jl role list`; a bare `jl role` runs it): query the index. Default
  output omits the large `description` to save tokens.
  `--employer` substring-matches the display employer (the older `--company` name
  is a deprecated alias, since "company" elsewhere means the canonical slug).
  `--remote` is tri-state: `--remote` requires remote, `--remote=false` requires
  on-site, omitting it matches either. `--lane <name>` filters by role type using
  the keyword map in `lanes.yaml` (see Lanes below).
- `jl role changes`: the delta from the **last import** specifically.
- `jl role show <id>` (alias `jl role get`): one role's full detail. Feeds
  `jl add --from-role`.
- `jl role rm <id>`: drop a junk role from the index by id.

#### Lanes

A "lane" is a named bundle of title keywords for filtering the index to a role
type, so a search that spans several titles ("SRE", "platform engineer",
"infrastructure engineer") is one `--lane reliability` flag. Lanes live in an
editable `lanes.yaml` in the data dir (seeded by `jl init` with a small set of
example lanes); each key is a lane name and its value is the list of
case-insensitive title substrings that match it. The shipped defaults are just
examples to edit, add to, or replace; `jl role ls --lane <name>` validates only
that the name exists as a key in the file.

### Company

- `jl company add|ls|set|rm|show` (alias `jl target ...`; a bare `jl company`
  runs `ls`): manage `companies.yaml` (name to `{status, ats, slug,
  careers-url}`, used when invoking the scraper). Each company has a `status` you
  set: `active` (in the `jl fetch` rotation) or `paused` (tracked, skipped on
  fetch); new companies default to active, and data written before the field
  reads as active. `add` accepts one or more careers/posting URLs (a board root
  or a specific posting; `-` reads URLs from stdin) and parses the ATS and slug
  from recognized multi-tenant hosts, falling back to `--name --ats --slug
  --careers-url` for a custom board; it eager-scaffolds
  `companies/<slug>/company.md`. `ls` shows `STATUS` plus data columns `ROLES`
  (open roles) and `APPLIED` (log entries linked to the slug); it lists only
  active companies unless `--all`. `set <name> active|paused` changes the status.
  `show <name>` prints status, open roles, applications, and the research files in
  the folder.

### Resume

`jl resume` is a top-level collection: a base resume plus one tailored variant
per role. `jl` only extracts text; tailoring is the agent's job.

- `jl resume` / `jl resume ls`: list the base resume and every tailored-per-role
  variant (role id, company, title, source file). `--json` emits a struct array.
- `jl resume set <file.md|pdf|json>`: record the canonical base resume under
  `resume/` and write a plaintext `resume.txt` so an agent can read it cheaply
  without PDF tooling. PDF to text via `dslipak/pdf`; Markdown and JSON pass
  through. This is the resume `jl profile build` reads from.
- `jl resume add --role <id> <file>`: store a tailored variant for a role under
  `companies/<slug>/resume-<role>.{src,txt}`. One per role; a re-add overwrites.
- `jl resume show <id|base>`: print the extracted text.
- `jl resume diff <id>` / `jl resume diff <idA> <idB>`: unified diff of the
  extracted text (base vs a role, or two roles), computed on demand from the
  current `.txt` files. No stored versions; jl renders the diff itself (a small
  stdlib LCS line diff, no `diff` binary, no dependency).
- `jl resume rm <id>`: remove a tailored variant. The base is replaced via
  `jl resume set`, not removed.

A tailored resume can be linked to a log entry: `jl log add --from-role <id>`
auto-links the role's stored variant, and `--resume <id>` sets it explicitly.

### Profile

- `jl profile` / `jl profile show`: print `profile.md`.
- `jl profile build [<file>]` (alias group `jl narrative`): scaffold the root
  `profile.md` and `accomplishments.md`, then **emit a ready-to-run prompt on
  stdout** (the `skill/prompts/build-profile.md` instructions plus your
  `resume.txt`). An optional `<file>` sets the base resume first. Pipe it into
  any agent CLI:

  ```
  jl profile build | claude -p "build my profile"             # Claude Code
  jl profile build | codex exec "build my profile" < /dev/null # Codex CLI
  jl profile build | pi -p "build my profile"                # pi
  ```

  The agent reads the resume and writes the files. `jl` composes the prompt; the
  agent writes the files. `jl` never reads or understands the resume itself.
- `jl profile edit`: open `profile.md` in `$EDITOR` (the by-hand path, no AI).
- `jl profile prompt`: emit only the raw build-profile prompt to stdout (the
  clean pipe contract; replaces the old `profile init --print`).

### Setup

- `jl init`: scaffold the data directory (see above).
- `jl config <key> [value]` / `jl config set <key> <value>`: get or set config
  (active state, weekly minimum, resume path).
- `jl version`: print version.

## Scraper schema mapping

There are no board adapters in `joblog`. Scraping is the scraper's job (or a curl
export for session-gated boards); `jl role import` just ingests the resulting
JSON.

Mapping notes, verified against the jobhive source:

- jobhive's schema is **29 fields** (`global_id`, `url`, `title`, `company`,
  `ats_type`, `ats_id`, `location`, ..., `salary_summary`, `salary_min`,
  `salary_max`, `description`, `posted_at`, `fetched_at`, `raw`, and others).
  Unmarshal with matching `json` tags plus `,omitempty`, and ignore unknown fields
  for forward compatibility.
- `global_id` is **opaque**. It is nominally `{ats}:{ats_id}`, but `ats_id` may
  contain colons and can fall back to a UUID, so never parse it; treat it as a
  string key.
- The `scrape` feed is **unenriched**: `salary_min` / `salary_max` and `is_remote`
  are often null. Parse `salary_summary` ourselves for the comp we keep.
- pandas emits datetimes (`posted_at`, `fetched_at`) as **epoch-millis integers**,
  not RFC3339. Convert on import.

We keep only what we need (title, employer, location, remote, url, salary,
description). `companies.yaml` records `name` to `{ats, slug, careers-url}` for the
scraper.

## State compliance profiles

A **state is a compliance profile** that `joblog` tracks your activities against:
how many activities you owe per week, which activity types count, which fields
each activity must capture, and the report format the agency wants. One superset
`Entry` schema stores everything; a small per-state plugin holds the rules plus
the agency's render format.

```go
type State interface {
    Code() string                      // "tx","ca",...
    MinDefault() int                   // default required/week; 0 = "reasonable/unspecified"
    Submit() bool                      // true = submit weekly; false = keep + produce on audit
    FormName() string                  // official form id, for Render()
    Retention() string                 // how long to keep
    SourceURL() string                 // official state workforce page (printed as "verify here")
    Check(week []Entry, min int) (n int, ok bool)
    Render(week []Entry) string        // the agency's form/format
}
```

All 13 researched states are implemented: TX, CA, FL, NY, PA, IL, OH, GA, NC, MI,
NJ, VA, WA. The verified values to encode (primary sources, June 2026; full
citations live in code comments):

| State | Min/week | Submit? | Official form | Retention | Notes |
|---|---|---|---|---|---|
| **TX** | 3 floor (1-5 by county/letter) | keep | BN900E Work Search Activity Log | benefit year | reg WorkInTexas <=3 biz days, counts as 1 but not alone; pending Apr-2026 rule to floor 5, not yet adopted |
| CA | none (per-claimant notice) | keep | none (DE 429Z notice) | ~5 yr (Title 22) | CalJOBS registration |
| FL | 5 (3 in counties <=75k) | **submit (CONNECT)** | Work Search Record | 1 yr | Employ Florida reg |
| NY | 3 (different days) | keep | WS-5 | 1 yr | |
| PA | 2 apps + 1 activity (compound, substitution) | keep | UC-304 | 2 yr | CareerLink reg <=30 days |
| IL | none fixed ("multiple"; TRA=5) | keep | ADJ034F | 53 wk | no job-board browsing |
| OH | 2 | keep | Work-Search Activities Log (generic) | **18 mo** | |
| GA | 3 new contacts | **submit (MyUI/fax)** | DOL-2798 | unspecified | |
| NC | 3 (1 may be reemployment) | **submit (MyNCUIBenefits, before cert)** | online entry | up to 5 yr | statewide as of Dec 2025; new claims online, legacy keep |
| MI | **1** (to 3 Jul-2026) | **submit (MiWAM, blocks pay)** | in-MiWAM (no claimant form) | 2 yr | |
| NJ | 3 contacts ("reasonable", per BC-514 guidance) | keep | BC-514 | claim life | not from N.J.A.C. 12:17-4.3 |
| VA | 2 (diff employers; no blind ads) | **submit (VUIS/CSS; paper q4wk for phone)** | Work Search Record | 1 yr | hiring-authority contact |
| WA | 3 | keep | ESD job-search-log | 30 days post-BY | broad activities (videos/courses count) |

`tx` is built first and most carefully, reproducing the BN900E weekly block. The
interface seam keeps each other state to roughly one small file. Volatile values
(MI Jul-2026, the TX Apr-2026 proposal, the NC rollout) are flagged in code
comments to re-verify.

**Always show the source plus a disclaimer.** `jl report`, `jl report states`,
and setting a state print the known requirements (min, submit, retention, form),
followed by a line like: "Requirements change and can vary by county; verify at
<SourceURL>." We surface what we researched because it is genuinely useful, but we
never present it as authoritative. Each profile stores its primary-source URL.

The weekly minimum is per-claimant configurable (`jl config set min N`),
defaulting from the profile, because several states set it by county or by the
claimant's determination letter. Switching state with `jl config set state` warns
when a manual `min` override is still in effect, so the old number is not silently
applied to the new state.

**Week boundary.** The compliance week is currently Monday-to-Sunday for every
state. Some agencies define the benefit week differently (often Sunday-to-Saturday
or keyed to the claim's effective date); making the boundary per-state is a known
follow-up. Until then, confirm the week span shown in the report header matches
the state's, and use `--week <any-date-in-it>` to report a specific week.

## Agent-friendliness

- Every command takes `--json` for structured, parseable output.
- Errors go to stderr with a nonzero exit code.
- The CLI is fully non-interactive: no prompts, no spinners blocking stdin. It is
  safe to drive from a pipe or a headless agent.

## Testing strategy

Table-driven tests across the board, with **golden-file tests for each
`State.Render()`** against known-good fixtures (especially the TX BN900E). Compliance
output must be correct, so that rendering is the one area that genuinely needs
coverage. Run with `go test ./...`.
