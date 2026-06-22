# Prompt: suggest-roles

Rank imported roles against the user's profile and return a short, reasoned
shortlist. This is the judgment step after `discover`.

## Inputs

- The candidate roles: `jl role ls --since 7d --new` (or `--changed`, or
  filter with `--employer`, `--remote`, `--title`, `--search`, or `--lane` to
  narrow to a role type, e.g. `--lane reliability`; lanes are user-editable in
  `lanes.yaml`). Default list output omits big fields to save tokens; pull a
  single role's full detail with `jl role show <id>`.
- The user's wants: read `profile.md`, both who they are (strengths, target
  lanes, domains, seniority) and the "what I want next" section (titles, comp,
  location, dealbreakers).

## How to rank

Score each role on these axes, weighted by the "what I want next" section of
`profile.md`:

1. **Experience fit**: does their background match the role's core needs (lanes,
   domains, seniority)?
2. **Next-role wants**: does it match the target titles/lanes they want next,
   not just what they have done?
3. **Salary**: is it at or above their floor? Many scraped roles have no comp;
   note "comp unknown" rather than assuming.
4. **Location / remote**: remote, in-region, or requires a relocation they have
   said they will (or will not) do.
5. **Dealbreakers**: drop or flag anything that hits a hard dealbreaker
   (visa sponsorship, on-call, sector, etc.).

## Output

A ranked shortlist (best first). For each: role title, company, a one-line
**reason for the rank**, the matching lane, and any flag (comp unknown,
relocation, dealbreaker risk). Put dealbreaker-hitting roles in a separate
"skip" list with the reason. Keep it tight; the point is to save the user from
reading the whole market.

End by offering to triage the top one (`triage-role` mode) or track it
(`jl log add --from-role <id>`, only after the user decides).
