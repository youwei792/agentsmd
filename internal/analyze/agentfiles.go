package analyze

import (
	"os"
	"path/filepath"
	"strings"
)

// agentFileSpecs maps tool name → candidate file paths (root-relative).
var agentFileSpecs = []struct {
	Tool  string
	Paths []string
}{
	{"agents", []string{"AGENTS.md"}},
	{"claude", []string{"CLAUDE.md", ".claude/CLAUDE.md"}},
	{"gemini", []string{"GEMINI.md", ".gemini/GEMINI.md"}},
	{"copilot", []string{".github/copilot-instructions.md"}},
	{"cursor", []string{".cursorrules", ".cursor/rules"}},
	{"windsurf", []string{".windsurfrules"}},
	{"cline", []string{".clinerules"}},
	{"aider", []string{"CONVENTIONS.md"}},
}

func analyzeAgentFiles(f *Facts) {
	for _, spec := range agentFileSpecs {
		for _, p := range spec.Paths {
			full := filepath.Join(f.Root, p)
			if !exists(full) {
				continue
			}
			if isDir(full) {
				// e.g. .cursor/rules directory.
				entries := listDir(full)
				if len(entries) == 0 {
					continue
				}
				f.AgentFiles = append(f.AgentFiles, AgentFile{
					Path: p, Tool: spec.Tool, Bytes: dirSize(full), HasRef: false,
				})
				continue
			}
			content := readFile(full)
			f.AgentFiles = append(f.AgentFiles, AgentFile{
				Path:   p,
				Tool:   spec.Tool,
				Bytes:  len(content),
				HasRef: hasAgentsRef(content),
			})
		}
	}
}

// hasAgentsRef reports whether content imports or references AGENTS.md the
// way Claude Code / Gemini CLI imports work ("@AGENTS.md").
func hasAgentsRef(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "@AGENTS.md") || t == "AGENTS.md" ||
			strings.Contains(t, "see @AGENTS.md") || strings.Contains(t, "@AGENTS.md ") {
			return true
		}
	}
	return strings.Count(content, "@AGENTS.md") > 0
}

func dirSize(dir string) int {
	total := 0
	for _, name := range listDir(dir) {
		if fi, err := os.Stat(filepath.Join(dir, name)); err == nil && !fi.IsDir() {
			total += int(fi.Size())
		}
	}
	return total
}
