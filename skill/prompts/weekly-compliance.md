# Prompt: weekly-compliance

Check the user's unemployment work-search compliance for the week, and if they
are short, help them get compliant and draft the state report.

## What to do

1. **Check compliance**:
   ```
   jl report --check
   ```
   This returns the count of qualifying activities for the active state and an
   exit code (nonzero = short of the weekly minimum). The minimum is
   per-claimant configurable (`jl config set min N`) because several states
   set it by county or notice.
2. **If compliant**: confirm the count, then render the report the user keeps or
   submits:
   ```
   jl report
   ```
   Relay the official form name and the source URL it prints.
3. **If short**: tell the user how many more activities they need and **suggest
   specific, qualifying activities** for their state (job applications,
   networking, a workforce-office visit, a job fair, an approved course or video
   where the state counts it). Do not log them yourself as completed; the user
   does the activity, then you record it:
   ```
   jl log add --type <type> --employer <e> --title <t> --method <m> --job-type <j>
   ```
   Then re-run `jl report --check`.
4. **Draft the report** once compliant: run `jl report` and present the
   rendered output for the user to paste into the state portal or keep on file.

## Safety

The hard rules live in the Safety section of `SKILL.md` (canonical). In short:
you draft, the human certifies and submits the weekly filing themselves; and
`jl report` already prints the official form name, source URL, and the
"verify at <SourceURL>; not legal advice" disclaimer, so relay it as-is.
