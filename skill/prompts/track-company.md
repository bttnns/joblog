# Prompt: track-company

The user saw a company and wants it on their radar. Low pressure: look it up,
confirm its jobs are fetchable, add it, and pull its roles in. A tracked company
starts active (in the fetch rotation); the user can pause it later with
`jl company set <name> paused`.

## Inputs

A company name or a URL (a careers page, a homepage, or just "I saw acme-corp.example").

## What to do

1. **Identify the ATS and slug.** From the careers URL, recognize the platform
   and slug:
   - `boards.greenhouse.io/<slug>` or `job-boards.greenhouse.io/<slug>` -> greenhouse
   - `jobs.ashbyhq.com/<slug>` -> ashby
   - `jobs.lever.co/<slug>` -> lever
   - `apply.workable.com/<slug>` -> workable
   - `*.myworkdayjobs.com/...` -> workday; iCIMS -> icims
   If you were given only a marketing site (e.g. `acme-corp.example`), fetch the
   careers/jobs page and look for the real board it embeds or links to; many
   custom sites front a standard ATS. If there is genuinely no supported ATS,
   record `ats: custom` with the careers URL.

2. **Verify it is fetchable.** Run the scraper and confirm it returns roles:
   ```
   jobhive scrape <ats> <slug> --format json
   ```
   If the board is session-gated or bot-protected, ask the user for a browser
   **Copy as cURL** and run that instead. If nothing works, that is fine: still
   track the company as `custom` so it is on the radar. **Fetch failure is
   non-fatal**; do not let it block tracking.

3. **Add the company** (this also scaffolds `companies/<slug>/company.md`). For a
   recognized multi-tenant board, pass the URL and jl parses the ATS and slug:
   ```
   jl company add <careers-url>
   ```
   For a custom board jl does not recognize, pass the flags by hand:
   ```
   jl company add --name <slug> --ats <ats> --slug <slug> --careers-url <url>
   ```
   If the user said *why* it caught their eye, write that one line into the
   `## Why interested` section of `companies/<slug>/company.md`; that note is the
   signal `suggest-companies` learns from.

4. **Import its roles now** (verify + import on add):
   ```
   jobhive scrape <ats> <slug> --format json | jl role import - --company <name>
   ```
   Report the terse delta (N new). For a `custom` board you could not fetch, skip
   this step.

## Output

One or two lines: company tracked (ATS, slug), N open roles imported, and an
offer to rank the relevant ones (`suggest-roles`) or triage the best one
(`triage-role`). Do not track an application here; that is a separate,
human-approved step.
