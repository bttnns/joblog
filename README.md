# joblog (`jl`)

**Track the companies you want, see what's newly open, and stay
unemployment-compliant, from the command line.**

`jl` is a small, local-first CLI that tracks the companies you are interested in,
surfaces new and changed roles (scraped by a producer you run, see
[Install an ATS fetcher](#install-an-ats-fetcher-producer)), logs your job-search
activity, and generates your state's
weekly unemployment work-search report. Your data stays local in a gitignored
`private/` directory. `jl` does **not** scrape; a producer does.

The binary is `jl`; the project is "joblog". It is built on a few co-equal
principles, not one overriding goal: **deterministic** (code renders compliance,
not the model), **auditable** (plain local files you can read and diff),
**token-efficient** (the CLI remembers, diffs, filters, and formats so an agent
spends tokens only on judgment), **minimal-dependency**, and **agent-portable**
(the pipe is the contract).

## Quickstart

There are two ways to use `jl`: drive it yourself from the command line, or let an
AI agent drive it for you. Both start from the same base setup, and the agent is
entirely optional, steps 1 and 2 are a complete job-search workflow on their own.

### 1. Base setup

```
go install github.com/bttnns/joblog/cmd/jl@latest
uv tool install jobhive-py               # a scraper (one of several producers)
jl init                                  # scaffold the data dir
jl config set state tx                   # your state, for the weekly report
jl resume set ~/my-resume.pdf            # store it + make resume.txt
```

> For a PDF resume, install poppler's **`pdftotext`** for clean extraction (`brew
> install poppler`, `apt install poppler-utils`). Without it `jl` falls back to a
> built-in pure-Go reader that mangles some PDFs. Markdown or JSON resumes need no
> extra tooling.

### 2. Drive it yourself

Everything `jl` does is a plain command you can run by hand:

```
# "I saw a company" -> track it and pull its roles
jl company add https://boards.greenhouse.io/acmecorp   # parses ATS + slug from the URL
jl fetch acme-corp                       # scrape + import its roles

jl company ls                            # who you track; active vs paused
jl role ls --since 7d --new              # what's new to look at
jl add --from-role greenhouse:123        # track an application
jl report                                # this week's work-search report + compliance
```

> For the full list of commands, run **`jl --help`** (and `jl <command> --help`).

### 3. Let an AI agent drive it (optional)

If you'd rather not run the commands by hand, an AI agent can do it for you, but
this is purely optional; plenty of people skip it. `jl` never talks to an agent
itself: it makes no network calls and has no agent inside it. Instead, an agent
(Claude Code, pi, Codex) runs `jl` *for you* through its skill, ranking roles,
triaging postings, researching companies, and keeping you compliant, all on top of
the same commands above.

This is also the only path that needs a profile; if you drive `jl` yourself you can
skip it entirely. `jl profile build` doesn't call an agent: it just prints a prompt
built from your resume to stdout, which *you* pipe into whatever agent CLI you have.
The agent then writes your `profile.md`, the distilled identity it ranks and triages
roles against.

```
jl profile build | claude -p "build my profile"   # you pipe the prompt; the agent fills profile.md
```

See [Using with an AI agent](#using-with-an-ai-agent) to install the skill and for
the full set of things you can ask it to do.

## Tracking companies you're interested in

Adding a company to your list puts it in the scrape rotation and scaffolds a
`companies/<slug>/company.md` for research. Each company has a status *you set*:
**active** (fetched by `jl fetch`) or **paused** (tracked, skipped on fetch). Your
pipeline shows up as data columns (open roles, applications), not a derived label.
Pass a recognized careers URL to `jl company add` and it reads the ATS and slug
for you; for a custom board, pass `--name --ats --slug --careers-url`.

```
jl company ls                   # NAME / STATUS / ATS / SLUG / ROLES / APPLIED / CAREERS-URL
jl company show acme-corp       # status, open roles, applications, and research files
jl company ls --all             # include paused companies (default shows only active)
jl company set acme-corp paused # take it out of the fetch rotation
```

Your agent can drive the whole "I saw `acme-corp`" flow (look up the ATS,
confirm jobs are fetchable, add it, import its roles) via the `track-company`
skill mode, and propose new companies from your pattern via `suggest-companies`.

## The pipeline

```
producer (scrape) -> jl role import -> role ls/changes -> jl add -> jl report
```

`jl` never scrapes. The producer you pick (jobhive by default, set another with
`jl config set scraper`) is what fetches the roles and emits them as the ATS job
JSON `jl` knows how to import; `jl fetch` just shells out to it and ingests the
result. Filter the index with `jl role ls` (`--since`, `--new`/`--changed`/`--gone`,
`--employer`, `--remote`, `--title`, `--search`). To narrow to a role type, define
named keyword bundles ("lanes") in `lanes.yaml` and pass `--lane <name>`; `jl init`
seeds a few example lanes to edit or replace.

## Using with an AI agent

`skill/SKILL.md` is the reasoning layer on top of the deterministic CLI. It
teaches an agent to drive the whole pipeline on your behalf: track a company the
moment you notice it, rank new roles against your profile, triage a posting, research
a company before an interview, and keep you compliant each week. The agent prepares;
you review and submit. The CLI has no submit verb and makes no network calls, so it
structurally cannot act without you.

### Install the skill

**Claude Code** (symlink so edits stay in sync). Run this from a clone of the
repo (the skill ships with the source, not the `go install` binary):

```sh
git clone https://github.com/bttnns/joblog && cd joblog
ln -s "$PWD/skill" ~/.claude/skills/joblog
```

The file must live at `~/.claude/skills/joblog/SKILL.md`. After that, Claude Code
loads it automatically and you can invoke it with `/joblog` or just describe what
you want in natural language.

**pi** (`@mariozechner/pi-coding-agent`): symlink the folder to
`~/.pi/agent/skills/joblog/` then invoke with `/skill:joblog`.

**Codex**: reference `skill/SKILL.md` from the repo `AGENTS.md` (native auto-load
is unconfirmed). Pass prompts headless via `codex exec`.

### Build your profile

The profile is the distilled identity the agent ranks and triages roles against, so
build it first. `jl profile build` scaffolds `profile.md` + `accomplishments.md` and
prints a ready-to-run prompt on stdout; pipe it into any agent CLI and the agent fills
your profile from your resume:

```sh
jl profile build | claude -p "build my profile"             # Claude Code
jl profile build | codex exec "build my profile" < /dev/null # Codex CLI
jl profile build | pi -p "build my profile"                 # pi
```

`jl` composes the prompt; the agent writes the files. `jl` never reads or understands
the resume itself, and the exact agent flags vary by tool and version: the pipe is the
contract. To fill the files in by hand instead, run `jl profile edit`; for the raw
prompt with nothing appended, `jl profile prompt`.

### What you can ask it to do

| Mode | What to say |
|------|-------------|
| `track-company` | "I saw acme-corp.example, go figure it out" |
| `suggest-companies` | "Suggest more companies like the ones I track" |
| `build-profile` | "Build my profile from my resume" |
| `discover` | "What roles are new this week?" |
| `suggest-roles` | "Which of those are worth my time?" |
| `triage-role` | "Should I apply to this posting?" (paste URL) |
| `research-company` | "Research Acme Corp before my interview" |
| `weekly-compliance` | "Am I compliant this week?" |

## Install an ATS fetcher (producer)

`jl` does no scraping itself: it imports the ATS job JSON a producer emits. The
Quickstart installs the default, jobhive; `jl fetch` shells out to it, and you can
swap producers with `jl config set scraper`. Common producers:

- **jobhive** ([github.com/kalil0321/ats-scrapers](https://github.com/kalil0321/ats-scrapers),
  PyPI `jobhive-py`), the default, covering many ATS platforms:
  `uv tool install jobhive-py`
- **JobSpy** ([github.com/speedyapply/JobSpy](https://github.com/speedyapply/JobSpy))
  for board aggregators (LinkedIn, Indeed, Glassdoor).
- A browser **Copy as cURL** export for session-gated boards a scraper cannot
  reach unauthenticated:

```
curl '<copied request>' | jl role import - --company acme-corp
```

> **A note on scraping.** `jl` itself fetches nothing; it only ingests JSON you
> produce. How you obtain that JSON is your responsibility: automated scraping of
> job boards may violate their terms of service, and the choice of producer and
> how you run it is yours, not the project's.

## States supported (work-search activity tracking)

`jl` ships compliance profiles for 13 states: **TX, CA, FL, NY, PA, IL, OH, GA,
NC, MI, NJ, VA, WA**. Set your active state with `jl config set state <code>`.

Several states set the weekly minimum by county or by the determination letter you
received, so set your own with `jl config set min <N>`. Requirements change and
can vary by county; `jl` always prints the official source URL so you can verify.
`jl` surfaces what we researched but never presents it as authoritative, and is
structurally incapable of auto-submitting or certifying anything (it has no such
command and makes no network calls).

## Data & privacy

Everything personal lives in a gitignored `private/` directory (or the XDG data
path, `~/.local/share/joblog`, for public users). Nothing personal is committed.
The repo itself is public-safe: all examples here are synthetic (`acme-corp`).

`jl` itself makes no network calls, so on its own your data never leaves your
machine. The moment you bring in an AI agent (step 3 of the Quickstart), that
changes: piping `jl profile build` to an agent, or letting the skill drive `jl`,
sends your resume, profile, and whatever else the agent reads to that model. Your
privacy then depends entirely on the agent and provider you choose (their data
retention, training, and logging policies), not on `jl`. If that matters to you,
pick the agent accordingly or skip the AI path and drive `jl` by hand.

```
profile.md         who you are and what you want next
accomplishments.md master accomplishment prose plus remixable STAR beats
resume/            your resume and the generated resume.txt
companies/<slug>/  one folder per company: company.md + research
data/
  log.json         your work-search activity log
  roles.json       the deduped index of roles seen
  companies.yaml   companies to scrape, mapped to their ATS
config.yaml        active state, weekly minimum, resume path
```

- **`DESIGN.md`** is the full CLI spec.
- **`AGENTS.md`** is the playbook for an agent working in this repo.
- **`skill/SKILL.md`** is the agent skill: install it once, then ask your agent
  to drive the pipeline on your behalf (see [Using with an AI agent](#using-with-an-ai-agent)).
