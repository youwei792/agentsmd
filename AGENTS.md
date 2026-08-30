# AGENTS.md

Guidance for AI coding agents working in this repository.

agentsmd is a zero-dependency Go CLI that keeps AI agent instruction files
(AGENTS.md, CLAUDE.md, GEMINI.md) healthy: it generates a grounded AGENTS.md
from repository facts, validates every command and file reference, audits
quality and token cost, and bridges CLAUDE.md/GEMINI.md to AGENTS.md.

## Commands

All commands run from the repository root.

### Build

```bash
go build -o agentsmd .
```

### Testing

```bash
go test ./...
```

### Lint & static analysis

```bash
go vet ./...
gofmt -l .
```

### Try it end to end

```bash
go run . doctor .
go run . analyze .
```

## Code style

- Formatting is enforced by gofmt: run `gofmt -w` on touched files.
- Standard library only — no third-party dependencies. Detection that needs
  a parser (package.json is JSON) uses encoding/json; TOML/YAML uses the
  pragmatic line readers in internal/analyze/tomlish.go.
- Keep validation conservative: when the engine is not certain a reference
  is broken, it must stay silent. False positives destroy trust in `check`.
- Errors go to stderr with exit code 1; findings are data (see validate.Finding).

## Testing notes

Tests are table-driven and build fixture repositories in t.TempDir();
nothing reads the network or the host filesystem outside the fixture.

- internal/analyze_test.go — toolchain detection per ecosystem
- internal/mdutil_test.go — markdown command/path extraction
- internal/validate_test.go — the conservative-checking contract
- internal/cli_test.go — end-to-end: builds the binary and runs init → sync → doctor

Run a single package with `go test ./internal/validate/`.

## Gotchas & conventions

- This tool parses its own AGENTS.md in CI (the dogfood job) — every command
  documented here must actually work, `go run .` form included.
- Agentsmd supports flags after positional arguments (e.g. `agentsmd tokens
  . --json`) via normalizeArgs in internal/cli/cli.go; keep new flags listed
  in boolFlags/valueFlags or they will not parse.
- Windows path handling: always filepath.Join / filepath.ToSlash, never "/".

---

<!-- Generated with care by humans and agents. Validate with: agentsmd check -->
