# Prompt: discover

Scrape the user's tracked companies, import the results into jl, and surface what
is new. This is the "what changed in the market this week" sweep.

## What to do

1. Read the company list: `jl company ls`. Each entry is
   `name -> {status, ats, slug, careers-url}` plus its open-role and application
   counts. Prefer `jl fetch` to scrape and import every active company in one
   step; the manual loop below is the portable fallback.
2. For each company, scrape and import (a scraper fetches, jl ingests):
   ```
   jobhive scrape <ats> <slug> --format json | jl role import - --company <name>
   ```
   `--format json` is required (jobhive's default output is a non-pipeable text
   table). Each import prints a terse delta (N new / M changed / K gone).
3. For any company a scraper cannot reach (session-gated boards), ask the user
   for a browser **Copy as cURL**, run it yourself, and pipe the JSON:
   `<curl ...> | jl role import - --company <name>`.
4. Surface what is new across all companies:
   ```
   jl role ls --since 7d --new
   ```
   To see what changed since the previous import specifically, use
   `jl role changes`. Narrow to a role type with `--lane <name>` (lanes are
   defined in `lanes.yaml`) when the user only cares about one track.

## Output

Report a compact summary: per company, how many new and changed roles, and the
new role titles worth a look. Do not dump full descriptions; pull those only on
request via `jl role show <id>`. Hand off to the `suggest-roles` mode to
rank them.

## Notes

- Keep tokens low: rely on the CLI's delta and pre-filtered list output rather
  than re-reading the full market each run.
- Do not track applications here. Discovery is read-only; tracking is a separate,
  human-approved step (`jl log add --from-role <id>`).
