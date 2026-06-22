# Prompt: suggest-companies

Recommend new companies worth tracking, learned from the pattern the user already
gravitates to. This is how their company radar gets honed over time, distinct
from `suggest-roles` (which ranks open postings, not companies).

## Inputs

- The current list: `jl company ls --all` (each company, its STATUS of active or
  paused, its open role count, and how many applications you have logged for it).
  Use `--json` for structure.
- Why they care: read `profile.md` (lanes, domains, seniority, what they want
  next) and skim the `## Why interested` notes in `companies/<slug>/company.md`.
- The built-in catalog: `jl company search <query>` searches an embedded snapshot
  of about 40k public ATS companies. It makes no network call, ranks by name
  match, and prints NAME / ATS / SLUG / URL plus whether each is already tracked
  (active, paused, or untracked). Scope it with `--ats`, cap it with `--limit`,
  and add `--json` for structure.
- Optional housekeeping signal: companies you track with zero open roles and zero
  applications over a long stretch are prune candidates (pause or rm them).

## How to reason

1. **Infer the pattern.** From the tracked set plus the profile, name the throughline
   in your own words: sectors (AI labs, infra, fintech), company stage/size,
   tech, mission, comp band. The companies they keep adding are the training data.
2. **Find candidates that fit the pattern** and are **not already tracked**. Favor
   ones whose roles plausibly match the user's lanes and next-role wants. Use
   `jl company search` to confirm a candidate is in the catalog and to read off its
   exact `ats`/`slug` (the TRACKED column already tells you whether it is new to
   the user). This is offline, so search freely; you do not need to fetch any CSV.
3. For a candidate not in the catalog, work out the likely **ATS + slug** by hand
   so it is still ready to hand to `track-company`.

## Output

A short ranked list (best first). For each: company name, a one-line **reason it
fits the pattern**, and its `ats`/`slug` from the catalog (or "needs lookup" when
it is not in the snapshot). Then a one-line housekeeping note: which tracked
companies look dormant (no roles, no applications) and worth pausing or pruning.
End by offering to bulk-add the ones the user picks with
`jl company add <slug-or-url>...` (slugs come straight from the search results),
then run `track-company` to import their roles. Do not add anything automatically;
the human chooses.
