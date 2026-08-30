# Contributing

Thanks for considering a contribution! agentsmd is deliberately small and
has exactly two hard rules:

1. **Zero dependencies.** Pure Go standard library only. If a feature
   requires a third-party parser, the answer is a pragmatic line-based
   reader, not `go get`.
2. **Conservative checking.** `check` must never report a false positive.
   If a heuristic can't be made sure, it doesn't ship. A silent miss is
   better than a wrong alarm — wrong alarms get the tool uninstalled.

## Development

```bash
go build -o agentsmd .
go test ./...          # fixture-based, no network
go vet ./...
gofmt -l .             # must be empty
```

The repo parses its own AGENTS.md in CI (`dogfood` job). If you add a
command to this file, it must actually work.

## Adding a linter / package manager / framework

Detection lives in `internal/analyze/`. Add the smallest possible evidence
(a manifest file, a dependency name, a config section) and a test in the
matching `_test.go`.

## Adding a lint rule

Rules live in `internal/lint/lint.go`. Every rule needs: a stable `RULE-NAME`,
an honest severity (Err only when truly broken), and a test.

## Pull requests

- Keep PRs focused; one behavior change per PR.
- Update `CHANGELOG.md` under an "Unreleased" heading.
- CI must be green (tests + vet + gofmt + dogfood).
