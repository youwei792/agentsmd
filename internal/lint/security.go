package lint

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/youwei792/agentsmd/internal/mdutil"
)

// Security rules. Agent instruction files are a real attack surface: they
// are read verbatim by agents that execute commands, and they routinely
// leak real credentials because people paste environment setup into them.
// Both rules here are precision-first — the conservative-or-silent promise
// applies to security findings doubly, because a wrong alarm trains people
// to ignore real ones.

type secretRule struct {
	re       *regexp.Regexp
	minDist  int // minimum distinct runes in the candidate (placeholder filter)
	severity Severity
	label    string
}

// High-confidence credential shapes. Placeholders ("sk-xxxx...", "0"*32)
// fail the distinct-rune filter and stay silent.
var secretRules = []secretRule{
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`), 1, Err, "private key block"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}\b`), 10, Err, "AWS access key id"},
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,251}\b`), 12, Err, "GitHub token"},
	{regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{20,}\b`), 12, Err, "API key (sk-…)"},
	{regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{20,}\b`), 10, Err, "Slack token"},
}

// genericAssign catches "API_KEY = "..."" style assignments with a long,
// non-placeholder quoted value.
var genericAssign = regexp.MustCompile(`(?i)\b(api[_-]?key|secret|token|password|credential)[_a-z0-9]*\s*[:=]\s*["']([A-Za-z0-9+/=_.-]{16,})["']`)

var placeholderValue = regexp.MustCompile(`^[xX*0]+$|your[-_]?key|example|placeholder|<[^>]*>|\$\{`)

// risky command shapes documented as things agents should run.
var riskyRules = []struct {
	re    *regexp.Regexp
	label string
}{
	{regexp.MustCompile(`\b(?:curl|wget)\b[^|]*\|\s*(?:sudo\s+)?(?:ba|z|)sh\b`), "pipe-to-shell install"},
	{regexp.MustCompile(`\beval\b`), "eval"},
	{regexp.MustCompile(`\bchmod\s+(?:-R\s+)?777\b`), "chmod 777"},
	{regexp.MustCompile(`\brm\s+-rf?\s+(?:--\s+)?(?:/|~|\$HOME)(?:\s|/|$)`), "destructive rm -rf on / or ~"},
	{regexp.MustCompile(`\bsudo\b`), "sudo"},
}

// checkSecurity scans the whole document for credentials and the extracted
// commands for risky patterns.
func (l *linter) checkSecurity(content string) {
	// Credentials: whole-document scan.
	seen := map[string]bool{}
	for _, r := range secretRules {
		for _, m := range r.re.FindAllString(content, -1) {
			if distinctRunes(m) < r.minDist {
				continue
			}
			// AWS documentation convention: example credentials carry the
			// EXAMPLE marker. They are never real secrets.
			if strings.Contains(m, "EXAMPLE") {
				continue
			}
			key := r.label + ":" + m
			if seen[key] {
				continue
			}
			seen[key] = true
			l.out = append(l.out, Finding{
				Rule: "SECRETS-FOUND", Severity: r.severity,
				Message: "likely " + r.label + " in AGENTS.md — rotate it and move it to the environment",
				Hint:    "agent files are read verbatim by every agent session and copied into forks/PRs",
			})
		}
	}
	for _, m := range genericAssign.FindAllStringSubmatch(content, -1) {
		val := m[2]
		if placeholderValue.MatchString(val) || distinctRunes(val) < 10 {
			continue
		}
		key := "assign:" + val
		if seen[key] {
			continue
		}
		seen[key] = true
		l.out = append(l.out, Finding{
			Rule: "SECRETS-FOUND", Severity: Warn,
			Message: "credential-looking assignment (" + m[1] + "=…) in AGENTS.md — move it to the environment",
		})
	}

	// Risky commands: only fenced/inline commands, not prose.
	reported := map[string]bool{}
	_, cmds, _ := mdutil.Parse("AGENTS.md", content)
	for _, c := range cmds {
		for _, r := range riskyRules {
			if !r.re.MatchString(c.Cmd) {
				continue
			}
			key := r.label
			if reported[key] {
				continue // one finding per pattern, not per line
			}
			reported[key] = true
			l.out = append(l.out, Finding{
				Rule: "RISKY-COMMAND", Severity: Warn,
				Message: fmt.Sprintf("AGENTS.md:%d documents a %s — agents follow documented commands literally", c.Line, r.label),
				Hint:    "move one-off setup behind a reviewed script and reference that instead",
			})
		}
	}
}

func distinctRunes(s string) int {
	set := map[rune]bool{}
	for _, r := range s {
		set[r] = true
	}
	return len(set)
}
