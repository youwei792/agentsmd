// Package tokens estimates the context cost of agent instruction files.
package tokens

import (
	"os"
	"path/filepath"
	"strings"
)

// FileReport is the token estimate for one file.
type FileReport struct {
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	Chars  int    `json:"chars"`
	Tokens int    `json:"tokens"`
}

// Report is the aggregate context footprint.
type Report struct {
	Files []FileReport `json:"files"`
	Total int          `json:"total_tokens"`
	Chars int          `json:"total_chars"`
	Bytes int          `json:"total_bytes"`
}

// Estimate returns the token estimate for a document. We use the widely
// used ~4 chars per token heuristic for English prose with a small penalty
// for markdown structure, and ~3.5 for code (symbols tokenize worse).
func Estimate(content string) int {
	prose, code := splitCode(content)
	p := len(prose) / 4
	c := int(float64(len(code)) / 3.5)
	w := len(strings.Fields(content)) // never below word count
	if p+c < w {
		return w
	}
	return p + c
}

// splitCode splits fenced code blocks from prose.
func splitCode(content string) (prose, code string) {
	var pb, cb strings.Builder
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			cb.WriteString(line + "\n")
			continue
		}
		if inFence {
			cb.WriteString(line + "\n")
		} else {
			pb.WriteString(line + "\n")
		}
	}
	return pb.String(), cb.String()
}

// agentFiles lists the files that typically enter the agent context.
var agentFiles = []string{
	"AGENTS.md", "CLAUDE.md", "GEMINI.md",
	".github/copilot-instructions.md", ".cursorrules", ".windsurfrules",
	".clinerules", "CONVENTIONS.md",
}

// Measure builds a report for the repository at root.
func Measure(root string) *Report {
	r := &Report{}
	add := func(rel string) {
		full := filepath.Join(root, rel)
		b, err := os.ReadFile(full)
		if err != nil || len(b) == 0 {
			return
		}
		content := string(b)
		r.Files = append(r.Files, FileReport{
			Path:   rel,
			Bytes:  len(b),
			Chars:  len([]rune(content)),
			Tokens: Estimate(content),
		})
	}
	for _, f := range agentFiles {
		add(f)
	}
	// .claude/CLAUDE.md and .cursor/rules/*.mdc
	add(".claude/CLAUDE.md")
	// Agent Skills bundles
	if ents, err := os.ReadDir(filepath.Join(root, ".claude", "skills")); err == nil {
		for _, e := range ents {
			if e.IsDir() {
				add(filepath.Join(".claude", "skills", e.Name(), "SKILL.md"))
			}
		}
	}
	if ents, err := os.ReadDir(filepath.Join(root, "skills")); err == nil {
		for _, e := range ents {
			if e.IsDir() {
				add(filepath.Join("skills", e.Name(), "SKILL.md"))
			}
		}
	}
	if ents, err := os.ReadDir(filepath.Join(root, ".cursor", "rules")); err == nil {
		for _, e := range ents {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".mdc") {
				add(filepath.Join(".cursor", "rules", e.Name()))
			}
		}
	}
	for _, f := range r.Files {
		r.Total += f.Tokens
		r.Chars += f.Chars
		r.Bytes += f.Bytes
	}
	return r
}

// ContextBudgets are common model context sizes for framing.
func ContextBudgets() []struct {
	Name string
	Size int
} {
	return []struct {
		Name string
		Size int
	}{
		{"128k", 128 * 1000},
		{"200k", 200 * 1000},
		{"1M", 1000 * 1000},
	}
}
