# Changelog

All notable changes to this project are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/) and the
project adheres to [Semantic Versioning](https://semver.org/).

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
