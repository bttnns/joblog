# jl data directory

This directory holds your personal job-search data. It is created and read by
`jl`. Nothing here should be committed to a public repository.

## Layout

```
profile.md         who you are and what you want next
accomplishments.md master accomplishment prose plus remixable STAR beats
resume/            your resume (resume.json/pdf/md) and the generated resume.txt
companies/         one folder per company: research + tailored material
data/
  log.json         your work-search activity log (the entries jl tracks)
  roles.json       the deduped index of roles seen, with new/changed/gone status
  companies.yaml   companies to scrape, mapped to their ATS, for the scraper
config.yaml        active state, weekly minimum, resume path
```

## Getting started

```
jl config set state tx              # set your state for compliance reports
jl resume ~/my-resume.pdf           # store your resume and make resume.txt
jl profile init | claude -p         # let an agent build your profile
jl add --employer "Acme" --title "SRE" --url https://...   # log an application
jl report                           # this week's work-search report
```

Run `jl --help` for the full command list.
