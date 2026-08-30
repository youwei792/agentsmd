// Package lint audits the quality of agent instruction files and produces
// an actionable score.
package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/youwei792/agentsmd/internal/analyze"
	"github.com/youwei792/agentsmd/internal/tokens"
	"github.com/youwei792/agentsmd/internal/validate"
)

// Severity of a finding.
type Severity string

const (
	Err  Severity = "error"
	Warn Severity = "warning"
	Info Severity = "info"
)

// Finding is one lint result.
type Finding struct {
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Hint     string   `json:"hint,omitempty"`
}

// Report is the full lint result.
type Report struct {
	Findings []Finding `json:"findings"`
	Score    int       `json:"score"`
	Grade    string    `json:"grade"`
}

type linter struct {
	root  string
	facts *analyze.Facts
	out   []Finding
}

// Run lints the agent instruction files of the repo at root.
func Run(root string, facts *analyze.Facts) *Report {
	l := &linter{root: root, facts: facts}
	content := readIfExists(filepath.Join(root, "AGENTS.md"))

	if content == "" {
		l.out = append(l.out, Finding{
			Rule: "AGENTS-MISSING", Severity: Err,
			Message: "no AGENTS.md found",
			Hint:    "run `agentsmd init` to generate one from repository facts",
		})
	} else {
		l.checkContent(content)
		l.checkSecurity(content)
	}
	l.checkToolFiles()
	l.checkStaleness(content)

	score := 100
	for _, f := range l.out {
		switch f.Severity {
		case Err:
			score -= 20
		case Warn:
			score -= 8
		case Info:
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	// A missing AGENTS.md is the dominant problem; reflect it honestly.
	if content == "" {
		score = min(score, 40)
	}
	return &Report{Findings: l.out, Score: score, Grade: Grade(score)}
}

// Grade maps a score to a letter grade.
func Grade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 60:
		return "C"
	case score >= 40:
		return "D"
	default:
		return "F"
	}
}

// ---- content rules ----

var sectionRe = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)

func (l *linter) checkContent(content string) {
	tr := strings.ToLower(content)
	tk := tokens.Estimate(content)

	// R: token budget.
	switch {
	case tk > 4000:
		l.out = append(l.out, Finding{
			Rule: "TOKEN-BLOAT", Severity: Warn,
			Message: fmt.Sprintf("AGENTS.md is ~%s tokens — agents skim long instructions", human(tk)),
			Hint:    "move long reference material into files the agent reads on demand",
		})
	case tk > 1500:
		l.out = append(l.out, Finding{
			Rule: "TOKEN-HEFTY", Severity: Info,
			Message: fmt.Sprintf("AGENTS.md is ~%s tokens", human(tk)),
			Hint:    "keep the always-loaded core lean",
		})
	}

	// R: missing sections.
	headings := map[string]string{}
	for _, m := range sectionRe.FindAllStringSubmatch(content, -1) {
		headings[strings.ToLower(strings.TrimSpace(m[1]))] = m[1]
	}
	hasSection := func(keywords ...string) bool {
		for h := range headings {
			hl := strings.ToLower(h)
			for _, k := range keywords {
				if strings.Contains(hl, k) {
					return true
				}
			}
		}
		return false
	}
	if !hasSection("command", "setup", "install", "build", "test") && !strings.Contains(tr, "```") {
		l.out = append(l.out, Finding{
			Rule: "NO-COMMANDS", Severity: Warn,
			Message: "no commands documented",
			Hint:    "agents work dramatically better when they know how to build/test",
		})
	}

	// R: vague advice.
	vague := 0
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if len(t) > 0 && len(t) < 30 && (strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ")) {
			body := strings.ToLower(strings.TrimPrefix(t, "- "))
			if isVague(body) {
				vague++
			}
		}
	}
	if vague >= 3 {
		l.out = append(l.out, Finding{
			Rule: "VAGUE-RULES", Severity: Info,
			Message: fmt.Sprintf("%d rules are too vague to act on (e.g. \"write good code\")", vague),
			Hint:    "replace with concrete, checkable instructions",
		})
	}

	// R: TODO leftovers.
	todos := strings.Count(strings.ToUpper(content), "TODO") + strings.Count(strings.ToUpper(content), "TBD")
	if todos > 0 {
		l.out = append(l.out, Finding{
			Rule: "PLACEHOLDERS", Severity: Info,
			Message: fmt.Sprintf("%d TODO/TBD placeholder(s) still present", todos),
			Hint:    "fill them in or delete them",
		})
	}

	// R: package manager mismatch.
	l.checkPackageManager(content)

	// R: duplicate headings.
	seen := map[string]int{}
	for _, m := range sectionRe.FindAllStringSubmatch(content, -1) {
		k := strings.ToLower(strings.TrimSpace(m[1]))
		seen[k]++
	}
	var dups []string
	for k, n := range seen {
		if n > 1 {
			dups = append(dups, k)
		}
	}
	if len(dups) > 0 {
		sort.Strings(dups)
		l.out = append(l.out, Finding{
			Rule: "DUP-SECTIONS", Severity: Info,
			Message: "duplicate section headings: " + strings.Join(dups, ", "),
		})
	}

	// R: undocumented commands — scripts that exist in the repo (and are
	// load-bearing for CI/dev) but never appear in the instructions.
	l.checkUndocumented(content)

	// R: dead command/file references (delegates to the validate engine).
	eng := validate.NewEngine(l.root, l.facts)
	for _, f := range eng.CheckDocument("AGENTS.md", content) {
		sev := Warn
		if f.Level == validate.Error {
			sev = Err
		}
		l.out = append(l.out, Finding{
			Rule: "BROKEN-REF", Severity: sev,
			Message: fmt.Sprintf("AGENTS.md:%d %s", f.Line, f.Message),
			Hint:    "fix the command or update AGENTS.md",
		})
	}
}

// checkUndocumented flags repo scripts with build/test/lint purposes that
// are nowhere mentioned in AGENTS.md. Agents copy what they can see: an
// undocumented canonical command tends to be reinvented badly.
func (l *linter) checkUndocumented(content string) {
	if l.facts == nil {
		return
	}
	important := map[string]bool{"test": true, "lint": true, "build": true, "format": true}
	var missing []string
	for _, s := range l.facts.Scripts {
		if !important[s.Purpose] {
			continue
		}
		if wordRe(s.Name).MatchString(content) {
			continue
		}
		missing = append(missing, s.Cmdline)
	}
	if len(missing) == 0 {
		return
	}
	sort.Strings(missing)
	if len(missing) > 3 {
		missing = append(missing[:2], "…")
	}
	l.out = append(l.out, Finding{
		Rule: "UNDOCUMENTED-CMDS", Severity: Info,
		Message: "repository commands never mentioned in AGENTS.md: " + strings.Join(missing, ", "),
		Hint:    "agents reinvent undocumented commands; document the canonical ones",
	})
}

// wordRe builds a case-sensitive word-boundary matcher per command name.
func wordRe(name string) *regexp.Regexp {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
}

var pmNames = []string{"npm", "pnpm", "yarn", "bun"}

var pmWordRes = map[string]*regexp.Regexp{
	"npm":  regexp.MustCompile(`\bnpm\b`),
	"pnpm": regexp.MustCompile(`\bpnpm\b`),
	"yarn": regexp.MustCompile(`\byarn\b`),
	"bun":  regexp.MustCompile(`\bbun\b`),
}

// negationRe catches lines that mention a PM only to forbid it
// ("don't use npm, use pnpm") — those are instructions, not confusion.
var negationRe = regexp.MustCompile(`(?i)\b(don'?t|do not|never|no |avoid|instead of|not use|not using|prefer|use \w+ ?(?:instead|over))\b`)

// mentionsPM reports whether content mentions the package manager in a
// positive (actionable) way: any line containing the word that is not a
// negation-only line.
func mentionsPM(content, pm string) bool {
	for _, line := range strings.Split(content, "\n") {
		if !pmWordRes[pm].MatchString(line) {
			continue
		}
		if negationRe.MatchString(line) {
			continue
		}
		return true
	}
	return false
}

func (l *linter) checkPackageManager(content string) {
	if l.facts == nil || !l.facts.Has("TypeScript/JavaScript") {
		return
	}
	pm := l.facts.PackageManager()
	if pm == "" || len(l.facts.PackageMgrs) > 1 {
		return // ambiguous — stay quiet
	}
	for _, other := range pmNames {
		if other == pm {
			continue
		}
		if mentionsPM(content, other) {
			l.out = append(l.out, Finding{
				Rule: "PM-MISMATCH", Severity: Info,
				Message: fmt.Sprintf("AGENTS.md mentions %q in a positive way but this repo uses %q (lockfile evidence)", other, pm),
				Hint:    "agents may pick the wrong install command",
			})
			return
		}
	}
}

// ---- tool files ----

func (l *linter) checkToolFiles() {
	if l.facts == nil {
		return
	}
	agents := l.facts.GetAgentFile("agents")
	claude := l.facts.GetAgentFile("claude")
	cursor := l.facts.GetAgentFile("cursor")

	if agents != nil && claude != nil && !claude.HasRef {
		l.out = append(l.out, Finding{
			Rule: "CLAUDE-NOT-SYNCED", Severity: Warn,
			Message: "CLAUDE.md exists but does not import AGENTS.md — Claude Code will read stale instructions",
			Hint:    "run `agentsmd sync`",
		})
	}
	if cursor != nil && agents != nil {
		l.out = append(l.out, Finding{
			Rule: "LEGACY-CURSOR", Severity: Info,
			Message: ".cursorrules/.cursor/rules exists; Cursor reads AGENTS.md natively now",
			Hint:    "consider migrating rules into AGENTS.md and deleting the legacy file",
		})
	}
}

// ---- staleness ----

var manifests = []string{"package.json", "go.mod", "Cargo.toml", "pyproject.toml", "Makefile", "justfile", "requirements.txt"}

func (l *linter) checkStaleness(content string) {
	agentsPath := filepath.Join(l.root, "AGENTS.md")
	if content == "" {
		return
	}
	agentsInfo, err := os.Stat(agentsPath)
	if err != nil {
		return
	}
	var newest time.Time
	var newestName string
	for _, m := range manifests {
		fi, err := os.Stat(filepath.Join(l.root, m))
		if err != nil {
			continue
		}
		if fi.ModTime().After(newest) {
			newest = fi.ModTime()
			newestName = m
		}
	}
	if newestName == "" {
		return
	}
	if agentsInfo.ModTime().Before(newest.Add(-14 * 24 * time.Hour)) {
		l.out = append(l.out, Finding{
			Rule: "STALE", Severity: Info,
			Message: fmt.Sprintf("AGENTS.md was last touched before %s changed — commands may have drifted", newestName),
			Hint:    "run `agentsmd check` to verify every command still works",
		})
	}
}

func isVague(body string) bool {
	vaguePhrases := []string{
		"write good code", "be nice", "do your best", "be careful",
		"think carefully", "use best practices", "follow best practices",
		"write clean code", "be concise", "be helpful", "work hard",
	}
	for _, v := range vaguePhrases {
		if strings.Contains(body, v) {
			return true
		}
	}
	return false
}

func readIfExists(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func human(n int) string {
	return fmt.Sprintf("%d", n)
}
