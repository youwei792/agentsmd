# Security Policy

## Scope

agentsmd is a static analysis tool for AI agent instruction files
(`AGENTS.md`, `CLAUDE.md`, `SKILL.md`, …). Its security posture is a design
property, not an afterthought:

- **No command execution.** `check`, `lint`, `doctor`, `analyze`, `skills`,
  `tokens` and `org` parse files and inspect the filesystem
  (`os.Stat`/glob) — they never execute the commands documented in the
  files they read. `init` and `sync` write files you explicitly ask for,
  inside the repository only.
- **No network access** in any code path except `agentsmd org`, which
  shells out to the `gh` CLI for read-only repository listing. Every other
  command is fully offline.
- **Zero third-party dependencies.** The entire supply chain is the Go
  standard library plus your compiler.
- **No telemetry.** Nothing is reported anywhere.
- **Checkout-only inspection.** References that escape the repository root
  (`../`) are never read.

## Supported versions

| Version | Supported |
|---|---|
| latest release | yes |
| older releases | no — upgrade |

## Reporting a vulnerability

Use GitHub's **Private vulnerability reporting** on the
[agentsmd repository](https://github.com/youwei792/agentsmd/security/advisories/new)
(do not open a public issue for security reports). Include the command
invocation, a minimal repository fixture, and the observed vs. expected
behavior.

Reports about false positives / false negatives of the checker are **not**
security issues — please open them in
[Discussions](https://github.com/youwei792/agentsmd/discussions) instead.

## Release integrity

Every release publishes `checksums.txt` (SHA-256 of all tarballs). The
[GitHub Action](action.yml) verifies the checksum before extracting or
running the binary. When installing manually, verify against it:

```bash
curl -fsSLO https://github.com/youwei792/agentsmd/releases/latest/download/checksums.txt
sha256sum -c checksums.txt --ignore-missing
```
