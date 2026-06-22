# Prompt: triage-role

Given one role (a URL or an imported role id), give a verdict on fit and tell
the user what to lead with if they apply. This answers "should I apply?"

## What to do

1. **Get the role.** If you have an imported id, `jl role show <id>` for full
   detail. If you only have a URL, identify the ATS and slug, scrape and import
   it first:
   ```
   jobhive scrape <ats> <slug> --format json | jl role import - --company <name>
   ```
   then `jl role show <id>`. For a session-gated board, use the browser
   Copy-as-cURL fallback.
2. **Read the user's context**: `profile.md` (who they are plus what they want
   next) and `accomplishments.md` (the master prose plus STAR beats).
3. **Probe the posting**: core responsibilities, required vs nice-to-have skills,
   seniority, comp (parse `salary_summary` if present), location/remote, and any
   dealbreaker signals.

## Output

1. **Verdict**: apply / maybe / skip, in one line, with the single biggest reason.
2. **Fit**: where the user is strong against this role, and the real gaps (be
   honest; do not oversell).
3. **What to lead with**: the 2-3 accomplishments and the specific STAR beat from
   `accomplishments.md` that map best to this posting's top needs.
4. **Next-role alignment**: does it move them toward what the "what I want next"
   section of `profile.md` says, or sideways?
5. **Flags**: comp, location, dealbreakers, visa, on-call, anything to confirm.

## Safety

See SKILL.md Safety (canonical): you draft and stage tailored material, the human
reviews and submits. Track only after they decide:
`jl log add --from-role <id> --status applied`.
