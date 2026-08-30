// Package generate builds a grounded AGENTS.md from repository facts.
// Every command it emits was detected in the repo; anything speculative is
// left as an explicit TODO for the human to fill in.
package generate

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/youwei792/agentsmd/internal/analyze"
	"github.com/youwei792/agentsmd/internal/tokens"
)

// Options controls generation.
type Options struct {
	Minimal bool
}

// Result describes what was generated.
type Result struct {
	Content  string   `json:"content"`
	Sections []string `json:"sections"`
	Detected []string `json:"detected"`
	TODOs    int      `json:"todos"`
	Tokens   int      `json:"tokens"`
}

// Build produces the AGENTS.md content for the given facts.
func Build(f *analyze.Facts, opt Options) *Result {
	var b strings.Builder
	var sections []string
	var detected []string
	todos := 0

	addSection := func(name string) {
		sections = append(sections, name)
		b.WriteString("\n## " + name + "\n\n")
	}
	todo := func(what string) {
		todos++
		b.WriteString("<!-- TODO: " + what + " -->\n")
	}

	detected = append(detected, f.Languages...)

	b.WriteString("# AGENTS.md\n\n")
	b.WriteString("Guidance for AI coding agents working in this repository.\n\n")

	if !opt.Minimal {
		b.WriteString(buildOverview(f))
	}

	// ---- Commands ----
	addSection("Commands")
	b.WriteString("All commands run from the repository root.\n\n")

	envCmds := commandsFor(f, "install")
	if len(envCmds) > 0 {
		b.WriteString("### Environment setup\n\n```bash\n")
		for _, c := range envCmds {
			b.WriteString(c + "\n")
		}
		b.WriteString("```\n\n")
	} else {
		todo("document how to set up the development environment")
	}

	buildCmds := commandsFor(f, "build")
	if len(buildCmds) > 0 {
		b.WriteString("### Build\n\n```bash\n")
		for _, c := range buildCmds {
			b.WriteString(c + "\n")
		}
		b.WriteString("```\n\n")
	}

	devCmds := commandsFor(f, "dev")
	if len(devCmds) > 0 {
		b.WriteString("### Development\n\n```bash\n")
		for _, c := range devCmds {
			b.WriteString(c + "\n")
		}
		b.WriteString("```\n\n")
	}

	testCmds := commandsFor(f, "test")
	if len(testCmds) > 0 {
		b.WriteString("### Testing\n\n```bash\n")
		for _, c := range testCmds {
			b.WriteString(c + "\n")
		}
		b.WriteString("```\n\n")
	} else {
		todo("document the test command for this repository")
	}

	lintCmds := commandsFor(f, "lint")
	formatCmds := commandsFor(f, "format")
	if len(lintCmds) > 0 || len(formatCmds) > 0 {
		b.WriteString("### Lint & format\n\n```bash\n")
		for _, c := range lintCmds {
			b.WriteString(c + "\n")
		}
		for _, c := range formatCmds {
			b.WriteString(c + "\n")
		}
		b.WriteString("```\n\n")
	}

	if other := commandsFor(f, "other"); len(other) > 0 && !opt.Minimal {
		b.WriteString("### Other\n\n```bash\n")
		for _, c := range other {
			b.WriteString(c + "\n")
		}
		b.WriteString("```\n\n")
	}

	// ---- Architecture / layout ----
	if !opt.Minimal {
		addSection("Repository layout")
		if f.Monorepo != nil {
			detected = append(detected, "monorepo: "+f.Monorepo.Kind)
			b.WriteString(fmt.Sprintf("This is a %s monorepo (see `%s`).\n\n",
				f.Monorepo.Kind, f.Monorepo.Manifest))
			if len(f.Monorepo.Packages) > 0 {
				b.WriteString("Workspace packages:\n")
				for _, p := range f.Monorepo.Packages {
					b.WriteString("- `" + p + "`\n")
				}
				b.WriteString("\n")
			}
		}
		for _, dir := range interestingDirs(f.Root) {
			b.WriteString("- `" + dir + "/`\n")
		}
		if len(interestingDirs(f.Root)) == 0 {
			todo("describe the repository layout")
		} else {
			b.WriteString("\n")
		}
	}

	// ---- Code style ----
	addSection("Code style")
	if len(f.Formatters) > 0 || len(f.Linters) > 0 {
		var bits []string
		if len(f.Formatters) > 0 {
			bits = append(bits, "Formatting is enforced by "+joinNames(f.Formatters)+".")
			detected = append(detected, f.Formatters...)
		}
		if len(f.Linters) > 0 {
			bits = append(bits, "Linting is enforced by "+joinNames(f.Linters)+".")
			detected = append(detected, f.Linters...)
		}
		b.WriteString(strings.Join(bits, " ") + "\n\n")
		b.WriteString("Run the lint and format commands above before declaring work done.\n")
	} else {
		todo("document code style conventions (naming, error handling, imports)")
	}

	// ---- Testing notes ----
	if !opt.Minimal {
		addSection("Testing notes")
		if len(f.TestFrameworks) > 0 {
			detected = append(detected, f.TestFrameworks...)
			b.WriteString("Tests use " + joinNames(f.TestFrameworks) + ".\n\n")
		}
		todo("note how to run a single test, and any tests that need network/database/docker")
	}

	// ---- Gotchas ----
	addSection("Gotchas & conventions")
	if f.Docker {
		detected = append(detected, "docker")
		b.WriteString("- Docker is used in this repo (compose files present); prefer the compose commands over installing services locally.\n")
	}
	if len(f.CICommands) > 0 && !opt.Minimal {
		b.WriteString("- CI runs (see `.github/workflows/` / `.gitlab-ci.yml`): keep these green.\n")
		b.WriteString("\n```bash\n")
		for _, c := range f.CICommands {
			b.WriteString(c + "\n")
		}
		b.WriteString("```\n\n")
	}
	todo("list project-specific conventions and things agents get wrong")

	b.WriteString("\n---\n\n")
	b.WriteString(fmt.Sprintf("<!-- Generated by agentsmd on %s. Validate with: agentsmd check -->\n",
		time.Now().Format("2006-01-02")))

	content := b.String()
	return &Result{
		Content:  content,
		Sections: sections,
		Detected: dedup(detected),
		TODOs:    todos,
		Tokens:   tokens.Estimate(content),
	}
}

// commandsFor returns unique command lines for a purpose, preferring explicit
// scripts (package.json/Makefile) over inferred ones.
func commandsFor(f *analyze.Facts, purpose string) []string {
	var out []string
	seen := map[string]bool{}
	// Explicit scripts first, in a stable order.
	var explicit []analyze.Command
	var inferred []analyze.Command
	for _, c := range f.Scripts {
		if c.Purpose != purpose {
			continue
		}
		if c.IsScript || c.Source != "" && (strings.HasSuffix(c.Source, "Makefile") || strings.HasSuffix(c.Source, "justfile") || c.Source == "Justfile" || c.Source == "makefile") {
			explicit = append(explicit, c)
		} else {
			inferred = append(inferred, c)
		}
	}
	sort.Slice(explicit, func(i, j int) bool {
		if explicit[i].Purpose != explicit[j].Purpose {
			return rankPurpose(explicit[i].Name) < rankPurpose(explicit[j].Name)
		}
		return explicit[i].Name < explicit[j].Name
	})
	for _, c := range append(explicit, inferred...) {
		if !seen[c.Cmdline] {
			seen[c.Cmdline] = true
			out = append(out, c.Cmdline)
		}
	}
	// Limit noise: at most 5 per purpose.
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// rankPurpose prefers canonical names (test before test:watch).
func rankPurpose(name string) int {
	switch name {
	case "test", "lint", "format", "fmt", "build", "dev", "start", "serve":
		return 0
	default:
		return 1
	}
}

func buildOverview(f *analyze.Facts) string {
	var b strings.Builder
	b.WriteString("## Project overview\n\n")
	langs := f.Languages
	var s string
	switch {
	case len(langs) == 0:
		s = "A software repository."
	case len(langs) == 1:
		s = "A " + langs[0] + " project"
	default:
		s = "A " + strings.Join(langs[:len(langs)-1], ", ") + " and " + langs[len(langs)-1] + " project"
	}
	if pm := f.PackageManager(); pm != "" {
		s += " using " + pm
	}
	for _, fw := range f.Frameworks {
		if isFrameworkWorthy(fw.Name) {
			s += ", " + fw.Name
		}
	}
	s += ".\n"
	b.WriteString(s + "\n")
	b.WriteString("<!-- TODO: replace with a one-paragraph description of what this project does -->\n\n")
	return b.String()
}

func isFrameworkWorthy(name string) bool {
	switch name {
	case "TypeScript", "ESLint", "Prettier", "Jest", "Vitest", "Mocha":
		return false
	}
	return true
}

var boringDirs = map[string]bool{
	"node_modules": true, ".git": true, "vendor": true, "dist": true,
	"build": true, "target": true, ".venv": true, "venv": true,
	"__pycache__": true, ".github": true, "testdata": true, "coverage": true,
}

func interestingDirs(root string) []string {
	var out []string
	for _, name := range analyze.ListTopDirs(root) {
		if !boringDirs[name] {
			out = append(out, name)
		}
	}
	if len(out) > 8 {
		out = out[:8]
	}
	sort.Strings(out)
	return out
}

func joinNames(s []string) string {
	s = dedup(s)
	if len(s) == 1 {
		return s[0]
	}
	return strings.Join(s[:len(s)-1], ", ") + " and " + s[len(s)-1]
}

func dedup(s []string) []string {
	seen := map[string]bool{}
	out := s[:0]
	for _, v := range s {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
