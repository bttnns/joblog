# Prompt: research-company

Deep-dive a company and write a research file plus fit notes. Use before an
interview or when deciding whether a company is worth pursuing.

## What to do

1. Research the company: what they do, engineering culture and stack, comp
   signal, remote posture, recent news, and anything that affects fit. Use web
   search and any links the user provides. Prefer primary sources (the company's
   own pages, filings, eng blog) over aggregators.
2. Read the user's context: `profile.md` (who they are and what they want next),
   so the fit section is specific to their lanes and wants.
3. Write the findings to `companies/<name>/research.md` in the data dir (the
   canonical per-company path; research and tailored material live together
   there, no separate research dir). Create the folder if needed.

## research.md structure

- **Snapshot**: what they do, in 2-3 lines.
- **Eng culture and stack**: languages, infra, how they build, team shape.
- **Comp signal**: bands or ranges if known; mark unknowns clearly.
- **Remote posture**: remote / hybrid / in-office, and where.
- **Fit for the user's lanes**: concretely, against `profile.md`'s target
  lanes: where they fit, where they do not, and the strongest angle.
- **Notes / risks**: visa sponsorship, fraud/phishing warnings on their domain,
  rebrands, hiring freezes, anything material.
- **Sources**: links at the end, every claim traceable.

## Output

Confirm the file path written, then give the user a tight verbal summary: the
one-line snapshot, the fit verdict, and the single best angle to lead with. If
they have an interview, hand off to `triage-role` for which accomplishments and
stories to bring.
