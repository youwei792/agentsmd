# Benchmark: validated against production AGENTS.md files

The promise of `agentsmd check` is **conservative or silent**: it never
reports a reference it cannot prove is broken. This document is the public
evidence for that claim, and the harness below lets anyone reproduce it.

## Method

1. Shallow-clone a diverse set of repositories that ship a production
   `AGENTS.md` (different languages, package managers, monorepo layouts).
2. Run `agentsmd check` on each.
3. **Manually triage every finding** against the actual repository state:
   a finding is correct only if the referenced command/path is genuinely
   absent AND the document presents it as runnable, not as an example or a
   runtime path.
4. Every false positive becomes a regression test before the next release.

## Results (v0.1.1, 2026-08-30)

| Repository | Stack | Refs checked | Real findings | False positives |
|---|---|---|---|---|
| [withastro/astro](https://github.com/withastro/astro) | pnpm monorepo, TS | 46 | 2 (cross-package relative paths, warnings) | 4 → fixed |
| [ollama/ollama](https://github.com/ollama/ollama) | Go + CMake | 7 | 0 | 1 → fixed |
| [fatedier/frp](https://github.com/fatedier/frp) | Go | 40 | 0 | 1 → fixed |
| [firecrawl/firecrawl](https://github.com/firecrawl/firecrawl) | pnpm monorepo | 13 | 0 | 4 → fixed |
| [excalidraw/excalidraw](https://github.com/excalidraw/excalidraw) | yarn, TS | 13 | 0 | 0 |
| [github/spec-kit](https://github.com/github/spec-kit) | Python | 46 | 0 | 9 → fixed |
| [Significant-Gravitas/AutoGPT](https://github.com/Significant-Gravitas/AutoGPT) | Python/TS monorepo | 66 | 0 | 66 → fixed |
| [NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent) | Python + TS | 324 | 2 (missing test file, warnings) | 202 → fixed |
| **Total** | | **~555** | **4** | **287 → 0** |

Findings that remain are warnings, verified by hand to be technically true
(references into package-relative paths that a root-relative checker cannot
resolve), and none of them fail CI.

## False-positive classes eliminated in v0.1.1

1. `cd` inside a fenced block not applied to later lines
2. leading `/` treated as filesystem-absolute instead of repo-root-relative
3. `./<name>` build artifacts flagged as missing files
4. runtime discovery dirs (`./.foo/`) flagged as missing
5. inline `./lib`-style shorthands flagged
6. conceptual mentions of `CLAUDE.md`/`README.md` treated as path references
7. template placeholders (`<package-dir>`) validated as literal paths
8. markdown table punctuation leaking into inline tokens; bare extensions
   (`.ts`) treated as paths; nested `package.json` fallback picking depth
   over proximity

Each class has a regression test in `internal/validate`/`internal/mdutil`.

## Reproduce

```bash
go install github.com/youwei792/agentsmd@latest
git clone --depth 1 https://github.com/ollama/ollama /tmp/ollama
agentsmd check /tmp/ollama        # expect: 0 errors
```

Add repositories, verify findings by hand, and open a PR with a row —
rows with unreproducible "real findings" will be removed. The benchmark
grows with the tool; new releases must not increase the false-positive
count on this corpus.
