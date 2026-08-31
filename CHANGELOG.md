# Changelog

All notable changes to this project are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/) and the
project adheres to [Semantic Versioning](https://semver.org/).

## [0.2.3] - 2026-08-31

### Fixed
- `sync` refuses to follow managed-target symlinks, returns non-zero on write
  errors, and makes `--mode copy` actually copy `AGENTS.md` idempotently.
- The composite GitHub Action now maps macOS to `darwin`, uses the real
  unversioned release asset names, installs `.exe` on Windows, verifies the
  checksum in a temporary directory, and defaults to its pinned release (or
  the latest release from a moving-major or unversioned ref).
- Multiline GitHub Actions `run:` extraction stops at the next YAML key, so
  generated shell blocks no longer contain `name:`, `uses:` or `with:` data.
- Common value-taking flags for Go, pytest, just and Yarn no longer become
  false missing-path/target findings; quoted values remain one argument.
- JSON modes preserve command failure exit codes for lint, tokens, skills and
  doctor.
- Binaries installed with `go install module@version` report the embedded Go
  module version instead of `dev` when no release linker flag is present.

### Changed
- `check` recursively validates scoped AGENTS.md/CLAUDE.md/GEMINI.md files,
  resolves their relative references from the document directory, and refuses
  to read instruction symlinks that resolve outside the repository.
- `doctor` includes the complete reference-validation report and cannot show
  a passing score when a sub-check has a hard failure.
- npm publishing now waits for the release job in the same workflow and can
  safely resume by skipping packages already published at that version.

## [0.2.2] - 2026-08-31 (npm only)

npm distribution is live: `npm install -g @momo792/agentsmd`.

- Main package `@momo792/agentsmd` + six platform packages
  (`agentsmd-{darwin,linux,windows}-{arm64,amd64}`), built from the
  checksummed release tarballs by `scripts/build-npm.sh`.
- Shim fix: resolve the platform binary across nested (scoped) and flat
  (unscoped) install layouts.
- The unscoped `agentsmd` name is blocked by npm's typosquat protection
  (`agents-md` exists); a support request may unlock it later.

## [0.2.1] - 2026-08-30

Review-round fixes.

### Fixed
- `init` no longer generates `npm run` commands for pnpm/yarn/bun
  repositories — generated instructions now use the repo's own package
  manager (`pnpm test`, `yarn test`, `bun run test`), so the generated
  file no longer trips the PM-MISMATCH lint rule on its own output.
- `doctor` now validates Agent Skills (`SKILL.md`) bundles as part of the
  report; invalid skills cost 15 points each.
- `agentsmd org` follows pagination (up to 500 repositories) instead of
  silently capping at the first 100.
- README "zero dependencies" badge anchor; dead code in lint exit path.
- Moving major tag `v1` now exists for `uses: youwei792/agentsmd@v1`.

## [0.2.0] - 2026-08-30

Security and surface-expansion release.

### Added
- `agentsmd skills`: validate Agent Skills (SKILL.md) bundles — frontmatter
  rules (name format & directory match, description length, allowed-tools),
  bundle integrity (referenced files must travel with the skill), token cost.
  Skill files are now also part of `tokens`/`doctor` context reports.
- `agentsmd org <owner>`: fleet report of AGENTS.md health across all public
  repositories of a GitHub org or user (read-only, via the `gh` CLI).
- Security rules in `lint`:
  - `SECRETS-FOUND` — live API keys, GitHub/Slack tokens, AWS key ids and
    private-key blocks in instruction files; placeholders (`sk-xxx…`) and
    the AWS `…EXAMPLE` convention stay silent.
  - `RISKY-COMMAND` — `curl … | sh`, `sudo`, `eval`, `chmod 777`,
    `rm -rf ~` documented as agent-runnable commands.
- `UNDOCUMENTED-CMDS` lint rule: repo commands that exist but are never
  mentioned in AGENTS.md.
- `checksums.txt` (SHA-256) published with every release; the GitHub Action
  verifies it before running the binary.
- Path hardening: references escaping the repository root (`../`) are never
  read.
- `SECURITY.md` and public accuracy benchmark ([docs/benchmarks.md]).

### Changed
- README available in English and 简体中文.

## [0.1.1] - 2026-08-30

Engine hardening release, driven by validating `check` against real
production AGENTS.md files (ollama, frp, firecrawl, excalidraw, spec-kit,
AutoGPT, hermes-agent — ~450 references scanned, zero real staleness found,
eight false-positive classes found in our own engine and eliminated).

### Fixed
- `cd` inside a fenced block now applies to later lines of the same block
  (`cd ui-tui` + `npm run dev` resolves against `ui-tui/package.json`).
- A leading `/` in a path reference is treated as repo-root-relative, not
  filesystem-absolute.
- `./<name>` commands matching the module/crate/package name are recognized
  as build artifacts (`./ollama serve` after `cmake --build`).
- Runtime discovery dirs documented as `./.foo/` are no longer flagged.
- Inline backticked `./lib`-style shorthands are no longer flagged.
- Conceptual mentions of `CLAUDE.md`/`AGENTS.md`/`README.md` as text are no
  longer treated as missing file references.
- Template placeholders (`<package-dir>`) in path references are skipped.
- Markdown table/list punctuation leaking into inline tokens
  (`docs/AGENTS.md)`) and bare extensions (`.ts`) are cleaned up.
- The nested `package.json` fallback now prefers the shallowest package.
- Self-reported `agentsmd version` now matches the release tag
  (injected at build time instead of a hardcoded constant).
- Windows: test binary gets a `.exe` suffix (CI fix).

## [0.1.0] - 2026-08-30

First public release.

### Added
- `init`: generate a grounded AGENTS.md from repository facts (package
  managers, scripts, Makefile/justfile targets, frameworks, linters, CI
  commands, monorepo layout), with `--minimal`, `--force`, `--dry-run`.
- `check`: validate that every command and file reference in agent
  instruction files exists; conservative-by-design, `--strict`, `--json`.
- `lint`: quality audit with A–F scoring (token bloat, dead references,
  package-manager mismatch, vague rules, placeholders, staleness).
- `tokens`: context-cost estimate across agent instruction files.
- `sync`: one-line `@AGENTS.md` bridges for CLAUDE.md/GEMINI.md with
  import/copy/symlink modes and a CI-friendly `--check`.
- `doctor`: all of the above in a single health report.
- `analyze`: show the detected toolchain facts.
- Composite GitHub Action for CI (`uses: youwei792/agentsmd@v1`).
- Release binaries for linux/darwin/windows on amd64/arm64.
