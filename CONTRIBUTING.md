# Contributing to joblog

Thanks for your interest. `jl` is a small, local-first Go CLI; contributions that
keep it small, dependency-light, and well-tested are very welcome.

## Development

```sh
go build ./...
go test ./...
go run ./cmd/jl --help
```

- **Format and vet** before sending a change: `gofmt -l .` should print nothing
  and `go vet ./...` should be clean.
- **Tests.** Logic lives in pure packages (`internal/dates`, `internal/roles`,
  `internal/worklog`, `internal/state`) that are table-testable without touching
  the filesystem; please add or update tests there. Compliance rendering is
  covered by golden files: when you change a `State.Render()`, regenerate the
  fixture with `go test ./internal/state/... -update` and confirm the diff is
  intended before committing.
- **The skill/CLI contract.** Every `jl ...` command written in `skill/*.md` is
  checked against the real command tree by a test, so renaming a command means
  updating the skill in the same change.

## Dependencies

`jl` deliberately ships with just two third-party dependencies (cobra and
`dslipak/pdf`); everything else is stdlib. Prefer stdlib, then a module already in
`go.mod`, and only then something new. If you add a dependency, justify it in the
PR and note its license.

## Scope and data

- Keep changes scoped and reviewable; honor `AGENTS.md` and `DESIGN.md`.
- **Never commit personal data.** Everything personal lives in a gitignored
  `private/` directory. Keep every example in code, docs, and fixtures synthetic
  (`acme-corp`, `example.com`).
- `jl` makes no network calls and does no scraping; please keep it that way. It
  ingests JSON a producer (run by the user) emits.

## Reporting issues

Open an issue with the `jl version`, your OS, the exact command, and what you
expected versus what happened. For compliance-data corrections, please include a
link to the primary state-workforce source.
