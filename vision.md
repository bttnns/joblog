# jl / joblog: whole-repo review, architecture, and product vision

Date: 2026-07-06. Scope: entire repo at commit a2f688b (clean tree), plus the idea,
the architecture, and forward-looking suggestions. Method: 13 independent review
angles (5 correctness, 4 cleanup, conventions, architecture, product, plus a core
split), one adversarial verifier per finding cluster, and a final gap sweep. Every
verifier verdict below was checked against quoted source; 42 of 42 verified claims
were confirmed, none refuted. Findings are ranked most severe first within each
section.

---

## 1. Executive summary

The engineering quality here is well above the side-project baseline: atomic
writes with fsync, a versioned store envelope, an advisory lock for human+agent
concurrency, a clean pure-core/IO-shell split in the domain packages, golden
tests per state, and a skill-to-CLI contract test. The product idea has a real
wedge (deterministic unemployment compliance rendering) and a genuinely novel
safety posture (no network, no submit verb, structurally).

The review found no rot, but it found three classes of problems that matter:

1. **Compliance correctness is live-broken in three places.** Michigan's weekly
   minimum is stale as of this month, NC and PA reject activities their own
   comments say should count, and the rendered state form can contradict the
   CLI's own compliance verdict when `min` is overridden. For a tool whose wedge
   is "trust the compliance output", these are the findings to fix first.
2. **Two data-loss paths and one lost-update race** in the store layer, all
   cheap to fix.
3. **The company-identity model is stringly and forks**, which breaks the
   README quickstart today and silently corrupts counts and gone-marking over
   time.

Everything below is actionable; a suggested fix accompanies each finding, and
section 7 turns them into a prioritized roadmap.

---

## 2. The idea (product review)

### Core idea and wedge

`jl` is a local-first system of record for a job search: track companies, ingest
role deltas from an external scraper, log every work-search activity, and render
the state's weekly unemployment work-search report deterministically from that
log. The user is a laid-off, terminal-native tech worker, most acutely one
drawing unemployment in one of the 13 supported states.

The sharpest wedge is the compliance report. It is a recurring, anxiety-laden,
legally consequential chore that nothing mainstream does well, and `jl report`
turns a plain activity log into the state's actual form. The discovery pipeline
(company tracking plus role deltas) is the retention loop; the agent skill is a
multiplier on both, not the wedge itself. The README's own framing agrees: steps
1-3 are complete without an agent.

### What is genuinely strong

- **Deterministic compliance rendering, tested like it matters.** Code renders
  the form, the model never formats it, and every state has a golden test. This
  is the correct call for output a state agency might audit, and it is a real
  differentiator versus "ask the LLM to draft my log".
- **Structural no-act safety.** No `net/http` import, no submit verb, no certify
  verb. The safety story is enforced by what the binary cannot do, not by prompt
  admonitions.
- **Token-efficiency as an explicit engineering principle.** Deltas instead of
  the whole market, `resume.txt` instead of re-parsed PDFs, list output that
  omits descriptions, a prebuilt profile. Most agent-adjacent tools hand the
  model everything; this one budgets.
- **The producer/consumer split.** "The pipe is the contract" keeps scraping
  maintenance, HTTP, and most ToS exposure out of the binary. The
  half-the-roles-gone import guard and `--no-gone` show real operational scar
  tissue.
- **Quietly excellent plumbing** (see architecture strengths below) and an
  honest privacy posture: the README says plainly that privacy ends where the
  agent pipe begins.

### Risks and hard questions

- **Stale compliance rules are the existential liability, and one is stale
  today.** `internal/state/mi.go` still returns a minimum of 1 while its own
  VOLATILE comment says the requirement rises to 3 in July 2026; today is
  2026-07-06, and MI is a submit-blocks-pay state. Provenance lives only in code
  comments, invisible to the user; nothing rendered says when a rule was last
  verified.
- **The shipped disclaimer is weaker than the one the docs promise.**
  `disclaimer()` prints only "Requirements change and can vary by county; verify
  at URL", but `skill/prompts/weekly-compliance.md` claims the output includes
  "not legal advice". The strongest liability-limiting words exist only in the
  skill and README, not in the artifact the user keeps.
- **The Monday week boundary is a known correctness hole.** Many benefit weeks
  are Sunday-Saturday; a Sunday activity bucketed into the wrong week can flip a
  verdict. DESIGN.md acknowledges this; for a compliance tool it is not a polish
  item.
- **Unsupported states hit a wall, not a ramp.** 37 states plus DC get "unknown
  state", even though `renderGeneric` plus a configurable minimum could serve a
  useful fallback today. The wedge feature is unavailable to roughly three
  quarters of the addressable market.
- **jobhive is a load-bearing third party you do not control.** The 29-field
  schema knowledge was verified once; there is no version pin, no schema sniff
  on import, no signal when it drifts. The pipe contract is the right hedge, but
  the happy path breaks silently on a single upstream release (and the
  zero-parse failure mode gone-marks roles; see finding C5).
- **The audience is a narrow, self-liquidating intersection**: CLI-comfortable,
  currently on unemployment, in 13 states, optionally agent-equipped, and the
  core user churns on success. That is fine if embraced: optimize for referral
  ("it got me through unemployment compliant"), not retention.
- **Onboarding friction stacks**: unsigned binary plus the Gatekeeper xattr
  dance, uv for jobhive, poppler for PDFs, and a skill installable only by
  cloning the repo and symlinking.
- **The ToS gray zone is delegated, not dissolved.** The skill affirmatively
  teaches the Copy-as-cURL workaround for session-gated boards. Personal-use
  scraping of public postings is widely tolerated, but how far the docs go in
  teaching the workaround deserves a conscious decision.

### Decide soon

- Rule provenance as first-class data (see suggestion R1).
- A versioned producer contract with checked-in fixtures (R4).
- Skill distribution: clone-and-symlink excludes every binary-only user. Embed
  the skill and add `jl skill install`, or ship it in the release archive.
- Distribution channel: a Homebrew tap removes both the PATH shuffle and the
  quarantine step; notarization later.
- Generic-state fallback versus researching 37 more states one by one.
- Template drift: the scaffolded data-dir README teaches dead commands (finding
  D2), and the build-profile prompt exists in two copies (`skill/prompts/` and
  `internal/assets/tmpl/`) that the repo `todo` already notes have drifted.

---

## 3. Architecture review

### Strengths

1. **Pure-core / IO-shell layering where applied.** `roles`, `worklog`, `dates`,
   `state`, and `catalog` are pure; `roles.Import(existing, payload, company,
   now)` is a deterministic function over copies, which makes the riskiest
   operation trivially unit-testable. `store` is the single persistence layer.
2. **Whole-file persistence done correctly for its scale.** `writeAtomic` does
   temp file + fsync + rename + parent-dir fsync; mutating commands take a
   blocking flock. Right-sized durability, no database.
3. **The store schema is already versioned** (envelope with `schema: 1`, newer
   files refused with an "upgrade jl" error, legacy bare arrays read as v0).
   Most projects this size skip this and regret it.
4. **The state package is a good plugin seam**: tiny Profile interface,
   self-registration with duplicate panic, shared render/qualify helpers, real
   unit tests for compound rules, golden files for output.
5. **The skill/CLI boundary has an actual contract test**
   (`TestSkillCommandsResolve` parses every command in SKILL.md and the prompts
   and resolves it against the live cobra tree). Unusually good idea.
6. **Safety is structural** (no HTTP, no submit/certify verbs) and
   **determinism is engineered** (nowFunc seam, sorted renders, stable
   tiebreaks, ISO dates, exit code 2 distinct from 1).

### Risks

1. **No expiry mechanism for compliance rules** (VOLATILE comments only; golden
   tests lock in what was encoded and cannot detect the world changing). MI is
   the live proof.
2. **The producer contract is informal** and its failure mode is silent for
   small companies (guard floor of 4; a parseable-but-alien payload equals "the
   company retired everything").
3. **`jl fetch` holds the exclusive lock across the whole network-bound loop**,
   blocking with no timeout and no message; the documented workflow is exactly
   "agent fetches while human logs".
4. **`internal/cli` is accreting domain logic**: `resume.go` (644 lines) holds
   the whole resume domain; company identity resolution, the destructive-import
   policy, and the profile heuristic all live in cli, reachable only by driving
   cobra against a temp dir.
5. **Company identity is stringly and forks**: display Name (the case-insensitive
   key), `model.Slug(Name)` (the linking slug), and `Company.Slug` (the ATS
   slug) can disagree; `role import --company <label>` derives identity from
   free text with no check against `companies.yaml`.
6. **Store growth is unbounded**: full descriptions kept forever, gone roles
   never pruned, raw payload archives accumulate per day per company, every
   import rewrites the whole indented file.
7. **Testability seams are half-built**: `--data-dir` and `nowFunc` are good,
   but output goes to `os.Stdout` globals, forcing the non-parallel-safe pipe
   swap in tests.
8. **A few silent error swallows** (`role changes` ignores LoadRoles failure,
   `company show` ignores load errors, the archive write is discarded). In a
   compliance tool, rendering from partial data silently is the wrong default.
9. **Skill drift coverage has gaps**: verbs are tested, flags and hardcoded
   data-dir paths are not; skill-vs-binary version skew is unmanaged.
10. **Monday week boundary for all 13 states** (acknowledged in DESIGN.md).

### Architecture suggestions (each with effort and payoff)

- **R1 (small): Verification metadata on Profile.** Add `VerifiedOn()` and
  optionally `ReverifyBy()`; print "rules verified 2026-06" in reports and warn
  on stderr when past due. Fix mi.go now. Turns rule maintenance from comment
  archaeology into an operational signal.
- **R2 (small): Make zero-parse imports loud.** If a non-empty payload array
  yields zero usable roles, or gone-marking would retire 100 percent of a
  company's open roles, refuse without `--force` regardless of the guard floor,
  reporting "parsed 0 of N elements".
- **R3 (small): Narrow the fetch lock and add feedback.** Scrape without the
  lock; lock only around each import (fan-out scrape / serial import is the
  safe shape since the flock is per-process). Try non-blocking flock first and
  print "waiting for another jl process" before blocking.
- **R4 (medium): Producer fixture corpus and a written contract.** Check in
  real jobhive payloads (and one curl export) under testdata, table-drive the
  import test over them, and document the fields `jl` actually reads as a
  versioned spec any producer can target.
- **R5 (small): Validate the `--company` label** against tracked companies
  (name or slug, case-insensitive); warn or resolve. Prevents identity forks.
- **R6 (medium): Extract `internal/resume`** with the same purity discipline as
  `roles`; move the destructive-import policy next to `roles.Import`.
- **R7 (small): Route output through cobra's writers** (`cmd.OutOrStdout()`),
  deleting the test pipe swap.
- **R8 (small): Data hygiene commands**: `jl role prune --gone-before 90d` and
  rotation for the import archive.
- **R9 (small): Extend the skill contract test to flags and paths**, and add a
  minimum-jl-version note to SKILL.md.
- **R10 (medium): Per-state week boundary** (`WeekStart() time.Weekday` on
  Profile, default Monday), threaded through `dates`. Do this before adding
  more states.

---

## 4. Verified correctness findings

Each finding was independently verified against quoted source (verdict noted).
Ranked by severity.

### Critical: compliance and data integrity

- **C1. Michigan's weekly minimum is stale right now.** CONFIRMED.
  `internal/state/mi.go:21` returns 1; the comment at mi.go:15 says the minimum
  is scheduled to rise to 3 in July 2026 and to re-verify after that date.
  Today is inside the window. A MI claimant with one logged activity gets
  "MEETS requirement" and exit 0 from `jl report --check` for a week the state
  may reject. Fix: set 3 (after re-verifying against MI's source), and ship R1
  so this class of bug becomes visible instead of silent.

- **C2. Store loaders silently destroy history on read errors.** CONFIRMED.
  `internal/store/store.go:137` (LoadLog) and :172 (LoadRoles):
  `if os.IsNotExist(err) || len(bytes.TrimSpace(b)) == 0` runs before
  `if err != nil`, so any non-ENOENT failure (permissions, I/O) reads as an
  empty store; the next `jl add` then atomically renames a one-entry file over
  months of legally required work-search records, no error shown. Fix: check
  `err` first, short-circuit only on `IsNotExist`.

- **C3. `openStoreForWrite` runs `Init()` before `Lock()`.** CONFIRMED.
  `internal/cli/cli.go:72-79`: the scaffold's exists-check-then-write runs
  unlocked, so on a fresh data dir two concurrent writers (the exact human+agent
  pair the lock's own docstring cites) can have one process's `SaveLog(nil)`
  seed clobber the other's just-committed entry. `Lock()` already does its own
  MkdirAll, so nothing forces this order. Fix: lock first, then Init.

- **C4. The rendered state form can contradict the CLI's compliance verdict.**
  CONFIRMED. `Profile.Render` (state.go:32) takes no minimum, every state
  passes `MinDefault()` to `renderGeneric`, and `header()` re-runs Check with
  that default, while `report.go:60-64` applies the `cfg.Min` override. With
  `min` set in config, `jl report` prints "Compliant: false" followed by a form
  body saying "MEETS requirement" (or the reverse). The audit-facing artifact
  misstates compliance. Fix: `Render(week, min)` or resolve min once and pass
  it through (pairs with the effectiveMin dedup, Q4).

- **C5. A `null` or zero-parse payload silently retires small companies.**
  CONFIRMED. `roles.go:217` unmarshals JSON `null` into a nil slice with no
  error and proceeds to gone-mark everything; the destructive-import guard
  (cli/roles.go:125) only engages at `pruneGuardFloor = 4` open roles, so
  companies with 1-3 open roles are wiped without warning when a scraper emits
  null or its schema drifts. Fix: R2.

- **C6. NC rejects activities its own rule says count.** CONFIRMED.
  `nc.go:29` uses `checkDefault`, whose qualify excludes workforce-office
  entries, while nc.go:11 says one of the three activities may be a
  reemployment activity such as an NCWorks workshop. A compliant NC week
  reports BELOW and `report --check` exits nonzero. Fix: NC-specific qualify.

- **C7. PA excludes CareerLink workshops from the "other activity" bucket.**
  CONFIRMED. `pa.go:41-49` counts the other bucket with `defaultQualify`, which
  returns false for workforce-office, while pa.go:15 names "a CareerLink
  workshop" as a valid example. (The stricter "3 applications alone do not
  pass" behavior is documented in pa.go:21 as an intentional floor, so only the
  workshop exclusion is a bug.) Fix: PA-specific other-bucket qualify.

- **C8. Re-running `jl init` on a legacy data dir hides every tracked
  company.** CONFIRMED. `store.go:341`: Init seeds an empty `companies.yaml`
  whenever absent, which permanently shadows LoadCompanies' `targets.yaml`
  fallback (only consulted when companies.yaml does not exist). Init is
  advertised as idempotent and safe to re-run. Fix: migrate legacy targets into
  the seed, or skip seeding when targets.yaml exists.

### High: identity, import, and locking

- **C9. Company lookups match display Name only; the README quickstart is
  broken.** CONFIRMED. Five sites (`fetch.go:130`, `company.go:80/332/371/406`)
  match `strings.EqualFold(c.Name, arg)` and never consult the slug. Adding
  `https://boards.greenhouse.io/acmecorp` derives Name "Acmecorp", so the
  README's next line, `jl fetch acme-corp`, errors with "no company named".
  Multi-word names also require shell quoting. Fix: one `findCompany` helper
  matching name or canonical slug (pairs with Q2).

- **C10. Slug-colliding company names cross-mark each other's roles gone.**
  CONFIRMED. Add dedup is by EqualFold Name, but import scoping is by
  `model.Slug(Name)`; "Acme Inc" and "Acme, Inc." coexist as rows yet share
  slug `acme-inc`, so alternating fetches flip each other's roles to gone and
  back. Fix: reject or merge slug collisions at add time.

- **C11. Path traversal via the import archive.** CONFIRMED.
  `roles.go:27-29` joins the raw `--company` label into
  `data/roles/imports/<date>/<company>.json`; neither `store.Path` nor
  `WriteFile` confines the result, so a label containing `../` writes outside
  the data dir, and the error is discarded (`_ =` at cli/roles.go:137). Local
  CLI, so severity is moderate, but agents pass labels programmatically. Fix:
  archive under `model.Slug(label)` like every other path.

- **C12. `jl fetch` holds the exclusive lock across the whole scrape, and a
  hung scraper blocks every jl command forever.** CONFIRMED. The flock is
  acquired before the loop (fetch.go:39), `lock_unix.go:19` uses blocking
  LOCK_EX with no timeout and no message, and `scraper.go:38` uses
  `exec.Command` with no context deadline. Fix: R3 plus a scrape timeout.

- **C13. Legacy roles (empty Company) can never be marked gone but are counted
  as prior-open.** CONFIRMED. Gone-marking (roles.go:294) requires
  `r.Company == companySlug` with no employer fallback, while
  `OpenCountForCompany` (roles.go:196) applies the fallback. Delisted pre-slug
  roles stay open forever and skew the guard denominator. Fix: apply the same
  fallback in gone-marking, or backfill Company on load.

- **C14. Paused companies are silently reactivated, and partial re-adds wipe
  fields.** CONFIRMED. The status-preserve branch (company.go:82) is dead code
  because every resolve path hardcodes `Status: active`; re-adding a paused
  company puts it back in the scrape rotation. And re-add replaces the record
  wholesale (company.go:85), so `jl company add --name acme` alone erases the
  stored ATS, slug, and careers URL. Fix: merge on re-add; treat flag-path
  status as unset.

### Medium: filters, deltas, and flows

- **C15. `jl role ls --new` is a silent no-op without `--since`, and fetch's
  own hint recommends exactly that.** CONFIRMED. `roles.go:354` applies the New
  predicate only when a Since cutoff exists; `fetch.go:103` prints "Review what
  is new with: jl role ls --new". The user gets all open roles presented as
  new. Fix: default `--new` to since-last-fetch (or last-import), or make the
  hint include `--since`.

- **C16. `--changed` misfires on every no-diff re-import.** CONFIRMED. Import
  bumps LastSeen on every upsert even with zero field changes (roles.go:264),
  and the filter uses `LastSeen != FirstSeen` (roles.go:366), so an identical
  payload imported a day later marks every role changed. Fix: track a real
  LastChanged date, or persist the last delta.

- **C17. A gone role that reappears is revived but reported nowhere.**
  CONFIRMED. Revival sets Status open (roles.go:273) but appears in neither
  New nor Changed, so `role changes` never surfaces relistings, which are
  exactly what a job seeker wants to see. Fix: add a Revived list (or count
  revivals as Changed).

- **C18. `jl fetch` always exits 0.** CONFIRMED (sweep, spot-checked).
  Scrape and import failures only increment a counter; RunE returns nil, so
  `jl fetch && jl role ls ...` pipelines proceed as if the sweep succeeded.
  Fix: nonzero exit when failed > 0 and fetched == 0 (or a `--strict` flag).

- **C19. An import is reported failed after it committed.** CONFIRMED.
  `SaveRoles` commits, then a `last-import.json` write failure returns an error
  (cli/roles.go:133), so fetch prints "import failed" for an applied import and
  `role changes` describes the wrong delta. Fix: treat the delta write as
  best-effort with a warning, like the archive write.

- **C20. `jl add --company` stores the raw label, breaking APPLIED counts.**
  CONFIRMED. add.go only slugs the employer fallback path; an explicit
  `--company "Acme Inc"` never matches the `acme-inc` key that appliedCounts
  compares. Fix: `model.Slug` the flag value. Related: **C21** (CONFIRMED)
  `jl update --employer` never re-derives Entry.Company and offers no
  `--company` flag, so corrected entries stay attributed to the old company.

### Lower severity (all CONFIRMED)

- **C22.** scraperArgv splits after substitution, so a slug containing spaces
  (reachable via percent-encoded URLs, which atsurl.go decodes and never
  validates) or starting with `-` injects extra argv/flags into the scraper
  invocation (argument injection, not shell injection). Fix: validate slugs to
  `[a-z0-9._-]` at parse/add time.
- **C23.** The `jobs.workable.com` host rule takes the first path segment, so
  aggregator links like `/view/<id>` track a bogus company "View"; pr4_test.go
  asserts the buggy output as expected. Fix: treat jobs.workable.com paths
  specially or reject them with the by-hand hint.
- **C24.** URL-derived slugs are not lowercased (atsurl.go:89), unlike the
  catalog path, so `boards.greenhouse.io/Acme` stores slug "Acme" and can 404
  on case-sensitive boards.
- **C25.** `jl config set state` prints none of the promised requirements
  summary or source-URL disclaimer (DESIGN.md promises it for state setup).
- **C26.** `jl company ls --json` and `jl config --json` emit PascalCase keys
  (yaml-only tags on the embedded structs) beside lowercase keys, breaking the
  snake_case JSON contract agents rely on. **C27.** `jl company rm`,
  `jl config set`, and `jl resume rm` accept `--json` but emit nothing on
  stdout.
- **C28.** `resume set` / `resume add` persist the new source file before text
  extraction, so a failed extraction leaves the new source paired with the old
  resume.txt, silently serving stale text. **C29.** pdftotext "missing",
  "failed", and "blank output" are indistinguishable and stderr is discarded, so
  real failures (password-protected PDFs) silently fall back to the weaker
  pure-Go extractor.
- **C30.** `filepath.Glob` on unescaped data-dir paths (add.go:124,
  resume.go:521) silently breaks tailored-resume linking when the path contains
  `[`, `?`, or `*`; errors discarded.
- **C31.** `$EDITOR` is passed whole as argv[0] (profile.go), so
  `EDITOR="code --wait"` always fails.
- **C32.** `--since` windows use `now.Add(-days*24h)` (DST-unsafe versus
  AddDate); material only when a window spans a DST transition and now is
  within the shifted hour of midnight. Real but marginal.
- **C33.** Lane keywords are matched case-sensitively against a lowercased
  title (roles.go:403-406), while lanes.yaml promises case-insensitive
  matching; any user-added keyword with an uppercase letter never matches.
- **C34.** NewID draws 4 random bytes with no dedup check, and exact-ID match
  returns the first hit; negligible at personal-log scale (~50 percent
  collision odds only near 77k entries) but a one-line dedup loop at add time
  closes it.
- **C35.** Data directories are created 0755 and the lock file 0644; file
  contents are 0600 only via CreateTemp's default. Other local users can list
  company names and resume variant filenames. Fix: 0700 for the data dir.
- **C36.** The release workflow's manual dispatch builds the branch HEAD (no
  `ref:` on checkout) while stamping the input tag, so a rebuilt release can
  publish binaries that do not match the tagged source. **C37.** `go install`
  builds always report version "dev" (no debug.ReadBuildInfo fallback), though
  README and SKILL.md tell users to verify with `jl version`.

### Docs and templates (all CONFIRMED)

- **D1.** DESIGN.md (lines 30 and 54), AGENTS.md:39, and CONTRIBUTING.md:28 all
  claim two third-party dependencies; go.mod has three direct requires
  (cobra, dslipak/pdf, go.yaml.in/yaml/v3). All three are properly vetted in
  ~/Dev/Docs/go-modules.md; only the count is stale.
- **D2.** The data-dir README scaffolded by every `jl init`
  (internal/assets/tmpl/datadir-README.md:24) documents two commands that do
  not exist: `jl resume <file>` (real: `jl resume set`) and `jl profile init`
  (real: `jl profile build`).
- **D3.** SKILL.md's data-dir resolution description omits that `./private` is
  used only when it already exists and that the real default is
  `~/.local/share/joblog`; an agent following it looks in the wrong place.
  DESIGN.md's precedence list has the same ambiguity (read literally, its step
  2 default makes step 3 unreachable; the code is sensible, the doc is not).
- **D4.** README.md:24 contains an en-dash (U+2013) in the steps range,
  violating the repo's own writing-style rule in AGENTS.md ("No em-dashes or
  en-dashes, anywhere"). The only dash hit in tracked files.
- **D5.** skill/prompts/weekly-compliance.md promises the report prints a
  "not legal advice" disclaimer; the binary never prints those words (see the
  product risk above).

---

## 5. Cleanup findings (reuse, simplification, efficiency)

All verified. These are quality items, not bugs; the first four have caused or
nearly caused real drift already.

**Reuse / single-source-of-truth**

- **Q1.** The company-identity fallback (use `.Company`, else
  `model.Slug(.Employer)`) is copied five times across four packages
  (company.go appliedCounts and openRoleCounts, roles.OpenCountForCompany,
  resume.go roleCompanySlug, qualify.go companyKey). One CompanySlug helper per
  model type ends the drift risk that would silently desynchronize APPLIED
  counts, compliance distinct-employer counting, and resume linking.
- **Q2.** The find-company-by-name loop is copied five times (see C9); one
  helper, matching name or slug, fixes the bug and the duplication together.
- **Q3.** The effective-minimum rule (`cfg.Min > 0` overrides MinDefault) is
  duplicated in report.go and status.go, with a comment admitting the mirror
  must be maintained by hand; C4 is the third copy of the same knowledge going
  wrong inside Render. One `effectiveMin(profile, cfg)` helper, used by all
  three.
- **Q4.** Of the 13 state files, 7 are pure data (constant methods plus
  delegation), and wa/ga/va differ only by qualify function; a table-driven
  profile struct with optional qualify/check/render hooks represents 11 of 13
  directly (ny/pa keep the explainer, tx keeps its custom Render). Roughly 200
  lines deleted and a copy-paste path for the next 37 states closed.
- **Q5.** update.go redefines add.go's ten entry flags (job-type help text has
  already drifted); findEntry and roles.Find implement two different
  exact-or-prefix matchers (one errors on ambiguity, the other returns silent
  not-found). Shared helpers, noting the intentional default differences.
- **Q6.** tx.go re-inlines render.go's week-span formatting byte for byte;
  extract weekSpan(). **Q7.** defaultLaneKeywords duplicates the embedded
  lanes.yaml word for word and its nil fallback is unreachable from the CLI;
  parse the embedded YAML instead. **Q8.** atsHostRules (URL to ats/slug) and
  catalog.URL (slug to URL) are two hand-synced tables in different packages
  with no round-trip test; derive both from one table. **Q9.** careersURLFor is
  a dead alias (zero callers); delete. **Q10.** zzfmt_test.go duplicates the
  Makefile's repo-wide gofmt gate for one package; delete (note: it is the only
  gate under bare `go test ./...`). **Q11.** pr4_test.go / pr5_test.go hold
  live feature tests under merged-PR names; redistribute by feature.
  **Q12.** LoadRoles/SaveRoles are line-for-line copies of LoadLog/SaveLog;
  one generic loader means schema migrations are written once (this matters
  the first time schemaVersion bumps).

**Efficiency** (matters at thousands of roles; all confirmed)

- **Q13.** `jl fetch` reloads and rewrites the entire roles.json once per
  company (importPayload does LoadRoles+SaveRoles inside the loop). Load once,
  thread the slice, save once.
- **Q14.** totalRolesForCompany re-parses the full payload just to count;
  return the count from Import (note the counts can legitimately differ by
  skipped-identity roles; surfacing "parsed N of M" also serves R2).
- **Q15.** Scrapes run sequentially; fan out the scrape step with a small
  bounded pool and keep imports serialized (the flock is per-process).
- **Q16.** `--search` builds a concatenated haystack and lowercases it plus the
  needle per row; lowercase the needle once and test fields individually.
- **Q17.** Post-import rendering and `role changes` call the linear
  roles.Find per id (O(delta x index), and the ids are full GlobalIDs so prefix
  tolerance is wasted); build a map once. Same shape in resume ls
  (sanitizeRoleID recomputed per variant per role) and resume diff (roles.json
  loaded twice).

---

## 6. Conventions

- The en-dash in README.md:24 (D4) violates the no-dash rule in both
  ~/.claude/CLAUDE.md and the home AGENTS.md; the repo's own AGENTS.md repeats
  the rule. Replace with a plain hyphen.
- The dependency-count claims (D1) should say three, or "three, of which one is
  the maintained YAML successor".
- Verified clean: all three go.mod deps are recorded in ~/Dev/Docs/go-modules.md
  with matching versions/licenses/dates, and ~/.claude/skills/joblog is a
  proper symlink to the repo skill directory, per the Dev AGENTS.md contract.

---

## 7. Prioritized roadmap

**Now (compliance and data safety; hours each)**

1. Fix MI minimum (C1) and re-verify the TX April-2026 proposal noted in tx.go.
2. Fix the store loader error swallow (C2) and the Init-before-Lock race (C3).
3. Thread the effective minimum into Render (C4 + Q3).
4. NC/PA workforce-office qualify fixes (C6, C7).
5. Refuse zero-parse and 100-percent-gone imports without --force (C5, R2).
6. Add "rules verified <date>" and "not legal advice" to the rendered
   disclaimer (R1, D5).
7. legacy targets.yaml migration in Init (C8).

**Next (identity and trust; a day or two each)**

8. One findCompany helper matching name or slug; fix the README quickstart
   (C9, Q2). Slug-collision rejection at add (C10). Merge instead of replace on
   re-add; stop reactivating paused companies (C14).
9. Slug validation at parse/add (C22, C24) and slugged archive paths (C11).
10. Narrow the fetch lock, add lock feedback, add a scrape timeout, and exit
    nonzero on total failure (C12, C18, R3).
11. Fix the scaffolded data-dir README and SKILL.md data-dir text (D2, D3);
    fix `role ls --new` semantics or the fetch hint (C15).
12. JSON key casing and missing --json outputs (C26, C27).

**Later (leverage)**

13. Table-driven state profiles (Q4), then a `generic` state profile to unlock
    all 50 states at reduced fidelity, then per-state week boundaries (R10).
14. Producer fixture corpus and versioned contract (R4); real LastChanged and
    a Revived delta (C16, C17).
15. Extract internal/resume (R6); cobra writers (R7); prune/rotation (R8);
    skill contract test for flags and paths (R9).
16. Distribution: embed the skill with `jl skill install`, Homebrew tap,
    ReadBuildInfo version fallback (C37), tag-ref checkout in the release
    workflow (C36).

---

## Appendix: top findings as JSON

```json
[
  {"file": "internal/state/mi.go", "line": 21, "summary": "MI weekly minimum still 1 though the file's own comment says it rises to 3 in July 2026, which is now", "failure_scenario": "MI claimant logs 1 activity the week of 2026-07-06; jl report --check exits 0 and prints MEETS requirement; the state expects 3", "verdict": "CONFIRMED"},
  {"file": "internal/store/store.go", "line": 137, "summary": "LoadLog/LoadRoles treat any non-ENOENT read error as an empty store because the empty-buffer check precedes the error check", "failure_scenario": "log.json unreadable (perms/IO); jl add sees an empty log, appends one entry, writeAtomic renames a one-entry file over months of work-search history", "verdict": "CONFIRMED"},
  {"file": "internal/cli/cli.go", "line": 73, "summary": "openStoreForWrite runs Init() before Lock(), an unlocked exists-then-write TOCTOU on fresh data dirs", "failure_scenario": "Concurrent jl add and jl role import on a fresh dir: one process's unlocked SaveLog(nil) seed renames over the other's just-committed locked save", "verdict": "CONFIRMED"},
  {"file": "internal/state/state.go", "line": 32, "summary": "Render has no min parameter, so the rendered form re-checks with MinDefault while the report banner uses the cfg.Min override", "failure_scenario": "config min: 5 over TX default 3 with 3 logged activities: banner says Compliant: false, BN900E body says MEETS requirement", "verdict": "CONFIRMED"},
  {"file": "internal/roles/roles.go", "line": 218, "summary": "JSON null payload parses as a valid empty import and gone-marks everything; the destructive guard only engages at 4+ prior open roles", "failure_scenario": "Scraper prints null with exit 0 for a company with 3 open roles; all 3 silently marked gone, reported as 0 roles (0 new, 0 changed, 3 gone)", "verdict": "CONFIRMED"},
  {"file": "internal/state/nc.go", "line": 29, "summary": "NC Check excludes workforce-office entries though its own rule text says one of the 3 may be a reemployment activity", "failure_scenario": "Week of [applied, applied, NCWorks workshop]: countQualifying=2, report --check prints BELOW and exits nonzero for a compliant week", "verdict": "CONFIRMED"},
  {"file": "internal/state/pa.go", "line": 44, "summary": "PA's other-activity bucket uses defaultQualify, which rejects the CareerLink workshops the file's own comment says count", "failure_scenario": "Week of [applied, applied, CareerLink workshop]: apps=2, other=0, compound check fails, false BELOW", "verdict": "CONFIRMED"},
  {"file": "internal/store/store.go", "line": 341, "summary": "Init seeds an empty companies.yaml, permanently shadowing the legacy targets.yaml fallback in LoadCompanies", "failure_scenario": "User with pre-rename data runs the advertised-idempotent jl init; company list, ATS mappings, and fetch rotation silently become empty", "verdict": "CONFIRMED"},
  {"file": "internal/cli/fetch.go", "line": 130, "summary": "Company lookups match display Name only, never slug, so the README quickstart's jl fetch acme-corp fails", "failure_scenario": "company add https://boards.greenhouse.io/acmecorp derives Name Acmecorp; jl fetch acme-corp errors: no company named acme-corp", "verdict": "CONFIRMED"},
  {"file": "internal/cli/company.go", "line": 80, "summary": "Add dedupes by EqualFold Name while import scopes by model.Slug(Name), so slug-colliding names cross-mark each other's roles gone", "failure_scenario": "Track Acme Inc (greenhouse) and Acme, Inc. (lever): both slug to acme-inc; each fetch retires the other company's roles, flip-flopping every cycle", "verdict": "CONFIRMED"},
  {"file": "internal/roles/roles.go", "line": 28, "summary": "ImportArchiveRelPath joins the raw --company label into the archive path; ../ escapes the data dir and the write error is discarded", "failure_scenario": "jl role import - --company '../../../../tmp/evil' writes the payload to /tmp/evil.json outside the data dir, silently", "verdict": "CONFIRMED"},
  {"file": "internal/cli/fetch.go", "line": 39, "summary": "The exclusive flock is held across the whole network-bound scrape loop with no timeout, and the scraper exec has no deadline", "failure_scenario": "A hung scraper during a 20-company fetch leaves LOCK_EX held forever; a concurrent jl add blocks silently until killed, losing the entry", "verdict": "CONFIRMED"},
  {"file": "internal/roles/roles.go", "line": 354, "summary": "role ls --new applies no predicate without --since, and fetch's printed hint recommends exactly the bare command", "failure_scenario": "After a fetch importing 3 new roles, jl role ls --new lists all 500 open roles as if new", "verdict": "CONFIRMED"},
  {"file": "internal/roles/roles.go", "line": 366, "summary": "--changed uses LastSeen != FirstSeen while Import bumps LastSeen on every no-diff upsert", "failure_scenario": "Identical payload re-imported the next day marks every re-seen role as changed; role ls --changed disagrees with role changes", "verdict": "CONFIRMED"},
  {"file": "internal/cli/company.go", "line": 82, "summary": "The status-preserve branch is dead code (every path hardcodes active) and re-add replaces the record wholesale", "failure_scenario": "Re-adding a paused company reactivates it into the scrape rotation; jl company add --name acme alone wipes its stored ATS, slug, and careers URL", "verdict": "CONFIRMED"}
]
```
