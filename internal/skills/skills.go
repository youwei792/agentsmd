// Package skills validates Agent Skills (SKILL.md bundles) so that
// agentsmd covers the second instruction surface that matters in 2026:
// not just repo-level AGENTS.md, but the skills the agent loads on demand.
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/youwei792/agentsmd/internal/mdutil"
	"github.com/youwei792/agentsmd/internal/tokens"
)

// Severity of a finding.
type Severity string

const (
	Err  Severity = "error"
	Warn Severity = "warning"
)

// Finding is one skill validation result.
type Finding struct {
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

// Report is the validation result for one SKILL.md.
type Report struct {
	Path     string    `json:"path"` // e.g. .claude/skills/pdf-tools/SKILL.md
	Dir      string    `json:"dir"`  // parent directory name
	Name     string    `json:"name"` // frontmatter name
	Describe string    `json:"description,omitempty"`
	Tokens   int       `json:"tokens"`
	Valid    bool      `json:"valid"`
	Findings []Finding `json:"findings"`
}

var nameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// skillDirs lists where SKILL.md bundles conventionally live.
var skillDirs = []string{".claude/skills", "skills"}

// Run validates every SKILL.md found under root.
func Run(root string) []Report {
	var out []Report
	for _, base := range skillDirs {
		entries, err := os.ReadDir(filepath.Join(root, base))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			rel := filepath.Join(base, e.Name(), "SKILL.md")
			content, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				continue
			}
			out = append(out, validate(root, filepath.ToSlash(rel), e.Name(), string(content)))
		}
	}
	return out
}

func validate(root, rel, dir, content string) Report {
	rep := Report{Path: rel, Dir: dir, Tokens: tokens.Estimate(content), Valid: true}
	add := func(sev Severity, msg string) {
		rep.Findings = append(rep.Findings, Finding{Severity: sev, Message: msg})
		if sev == Err {
			rep.Valid = false
		}
	}

	fm := parseFrontmatter(content)
	if fm == nil {
		add(Err, "no frontmatter block (--- delimited) found")
	} else {
		name := strings.TrimSpace(fm["name"])
		rep.Name = name
		if name == "" {
			add(Err, "frontmatter `name` is required")
		} else if !nameRe.MatchString(name) {
			add(Err, "name "+quote(name)+" must be lowercase letters, numbers and hyphens only")
		} else if len(name) > 64 {
			add(Err, "name exceeds 64 characters")
		} else if name != dir {
			add(Err, "name "+quote(name)+" does not match directory name "+quote(dir))
		}

		desc := strings.TrimSpace(fm["description"])
		rep.Describe = desc
		if desc == "" {
			add(Err, "frontmatter `description` is required")
		} else if len(desc) > 1024 {
			add(Err, "description exceeds 1024 characters")
		} else if len(desc) < 20 {
			add(Warn, "description is very short — say what the skill does and when to use it")
		}

		if at := strings.TrimSpace(fm["allowed-tools"]); at != "" && !allowedToolsRe.MatchString(strings.ReplaceAll(at, " ", "")) {
			add(Err, "allowed-tools "+quote(at)+" is not a comma-separated list of tool names")
		}
	}

	// Body file references are validated relative to the skill directory
	// (skills travel as self-contained bundles).
	skillRoot := filepath.Join(root, filepath.Dir(rel))
	_, _, pathRefs := mdutil.Parse(rel, content)
	for _, p := range pathRefs {
		if strings.HasPrefix(p.Path, "http") {
			continue
		}
		clean := strings.TrimSuffix(p.Path, "/")
		if clean == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(skillRoot, filepath.FromSlash(clean))); err != nil {
			add(Warn, fmt.Sprintf("SKILL.md:%d references `%s` which is not in the skill bundle", p.Line, p.Path))
		}
	}
	return rep
}

var allowedToolsRe = regexp.MustCompile(`^[A-Za-z0-9_:,-]+$`)

// parseFrontmatter returns key/value pairs of the YAML-ish frontmatter, or
// nil when the document has no frontmatter block.
func parseFrontmatter(content string) map[string]string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil
	}
	out := map[string]string{}
	for _, raw := range lines[1:] {
		line := strings.TrimSpace(raw)
		if line == "---" || line == "..." {
			break
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, ":")
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		val = strings.Trim(val, `"'`)
		if key != "" {
			out[key] = val
		}
	}
	return out
}

func quote(s string) string { return "`" + s + "`" }
