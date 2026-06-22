# jl redesign + discovery-first plan (ARCHIVED)

> **Status: shipped 2026-06-21.** Phases 0-2 merged to `main`. This is an archived
> planning record; the "Deferred follow-ups" at the bottom are being worked through
> separately. Kept for history, not as an active plan.

Working plan for the next iteration of this project. Decisions here were made in a
planning session and reviewed by two engineering passes (staff: idea/architecture;
senior: code/CLI). Delivery is **one PR per phase**, in order.

## Positioning

**Discovery-first.** The work-search compliance engine is solid and stays correct,
but the investment goes into making the role-discovery loop frictionless so the
roles index stops being empty in real use. The "companies I'm interested in" flow
is the on-ramp that populates it.

## Guiding principles (replaces the single "token efficiency" north star)

DESIGN.md drops the idea of one overriding goal and lists co-equal principles:

- **Deterministic** output (code renders; the agent does not format compliance).
- **Auditable** local store (whole-file artifacts, easy to inspect/diff).
- **Token-efficient** (compact, pre-filtered output; deltas not full market).
- **Minimal dependencies** (stdlib first; two third-party libs today).
- **Agent-portable** (Agent Skills standard; pipe contract as lowest common denominator).

## Mental model / command surface

Three nouns + one verb + setup. Everything ties together via a canonical company
**slug** shared by a company, its scraped roles, and your applications to it.

```
jl log    add | list | update | rm        # work-search entries (was bare add/list/update/rm)
jl report [--week --check]                # compliance form, top-level (the weekly payoff)
jl role   import | list | changes | get   # roles index (was `roles`, now singular)
jl company add | list | rm | show         # scrape list + per-company research (was `target`)
jl resume | profile | config | init | version
```

- Binary renamed `joblog` -> `jl` (`cmd/joblog` -> `cmd/jl`; sweep hardcoded name strings).
- Hidden bare-verb aliases (`jl add`, `jl list`) keep the daily path fast.
- `target` kept as a hidden alias of `company` for one release.
- `jl company list` shows a derived ENGAGEMENT column: `active` (you have a log entry
  for that slug) vs `watching` (in the list, no engagement yet). Presence in the list =
  interested; that is the entire "companies I'm interested in" model, no extra state.

## Data model change

Add a **canonical company key (slug)** to `Entry` and `Role`, set at write-time
(`jl add` / `jl role import`). One-time backfill slugifies existing records' employer.
"Active engagement" and roles gone-detection both key off this slug, never free-text
`Employer`. This fixes the gone-marking bug (below) and makes the company feature solid.

## File layout (after consolidation)

```
config.yaml                 state, weekly min, resume path
profile.md                  who you are + what you want next (profile.md + preferences.md merged)
accomplishments.md          master prose + STAR beats (accomplishments.md + stories/ merged)
resume/                     resume.txt + original
companies/<name>/company.md scrape metadata + research; eager-scaffolded on `company add`
data/
  log.json
  roles.json
  companies.yaml            scrape list (was targets.yaml; migration shim reads old)
README.md
```

Flatter than today: no `narrative/` folder, no dead `data/roles/` scaffold dir.

---

## Phase 0 - Foundation (PR 1)

De-risk the refactor and fix the one live correctness bug.

1. **Kill global flag state.** Move `flagJSON`/`flagDataDir` off package vars onto a
   config struct threaded via `cmd.Context()` (`internal/cli/cli.go:13-14,35-36`). Do
   first; unblocks safe command nesting.
2. **Canonical company slug** on `Entry` + `Role`, set at write-time; one-time backfill.
3. **Fix gone-marking** to scope by the import company slug, not
   `EqualFold(Employer, --company)` (`internal/roles/roles.go:201-214`). Add a
   regression test where `--company` differs from the payload `company`.
4. **Distinct exit codes**: "compliant but short" != tool failure
   (`cmd/.../main.go`, `internal/cli/report.go:65-76`).
5. **Close test gaps**: `report` / `report --check` exit-code contract, `resume`,
   `narrative`, `company`, and `--json` output shapes.

## Phase 1 - Restructure (PR 2)

1. **`joblog` -> `jl`**: rename `cmd/joblog` -> `cmd/jl`, root `Use`, centralize and
   sweep every hardcoded `joblog` string in help/examples.
2. **Noun grouping** per the surface above; hidden bare-verb aliases; `target` -> hidden
   alias of `company`. Leverage the existing `addCommand` registry.
3. **`targets.yaml` -> `companies.yaml`** (yaml key `targets:` -> `companies:`, `Target`
   -> `Company` struct, one-time migration shim that reads the old file if the new one is
   absent). `jl company add` eager-scaffolds `companies/<name>/company.md`;
   `jl company show <name>` reads it back.
4. **File consolidation**: drop dead `data/roles/` from `Init` (move archive +
   `last-import.json` path-building into `store` methods); merge `accomplishments.md` +
   `stories/` -> one file; merge `profile.md` + `preferences.md`; flatten `narrative/`
   to the data-dir root.
5. **Safety nets**: extend `TestCommandTree`; add a **skill<->CLI contract test** that
   greps every `jl ...` invocation in the skill prompt files and asserts it resolves.

## Phase 2 - Discovery-first feature (PR 3)

1. **Producer-agnostic (docs-only)**: document jobhive (default) + curl + JobSpy as
   first-class JSON producers; no import code change. Verify-fetchable-on-add is non-fatal.
2. **`track-company` skill mode**: company/URL -> identify ATS+slug -> verify fetchable
   -> `jl company add` -> import now.
3. **`suggest-companies` skill mode**: infer the user's pattern from the company list +
   profile, recommend new companies; label each `active` vs `watching`.
4. **Docs + skill**: update `SKILL.md`, all prompt files, `DESIGN.md`, `README.md`;
   collapse duplicated safety prose to one canonical place.
5. **Live demo**: add `mechanize.work` as the first tracked company (also exercises the
   non-ATS fallback path).

---

## Deferred follow-ups (from the reviews)

Most of these shipped 2026-06-21 (commits fdc373e, 3206e6d, b5ba979).

P1:
- [x] Harden `State.Check` for distinct-employer (GA, VA) and distinct-day (NY) states;
  counting keyed off the canonical company slug so repeats do not over-credit.
- [ ] Make the compliance week boundary explicit per-state (still global Monday-start;
  this is a Profile-interface change touching every state + `report`).

P2:
- [x] `writeAtomic` fsync (file + parent dir).
- [x] Normalize the URL-fallback role key (stable identity across re-scrapes).
- [x] Require `--company` on import; reject empty-employer roles.
- [x] Unify `--since` semantics between `jl log list` and `jl role list` (`dates.OnOrAfter`).
- [x] `jl role import` TTY-with-no-stdin guard.
- [x] `Import` no longer mutates its input slice.
- [ ] Test `pdfToText` (needs a committed PDF fixture; fiddly, lowest-value).
- [ ] `--remote` tri-state ergonomics (minor).

## Safety (unchanged, structural)

The tool never submits an application and never certifies unemployment. This is enforced
by the binary having no such verbs and making zero network calls, not by prompt wording.
`jl report` always prints the official state source URL plus a "verify here; not legal
advice" disclaimer (enforced in `render.go`).
