package analyze

import (
	"path/filepath"
	"regexp"
	"strings"
)

var ciRunRe = regexp.MustCompile(`(?m)^\s*(?:-\s+)?run:\s*(\|?.*)$`)

func leadingIndent(s string) int {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return i
		}
	}
	return len(s)
}

func isYAMLBlockScalar(s string) bool {
	switch s {
	case "|", "|-", "|+", ">", ">-", ">+", "":
		return true
	default:
		return false
	}
}

func analyzeCI(f *Facts) {
	// GitHub Actions.
	for _, wf := range findFiles(f.Root, "*.yml", "*.yaml") {
		if !strings.HasPrefix(filepath.ToSlash(filepath.Clean(wf)), ".github/workflows/") {
			continue
		}
		extractCIRuns(f, readFile(filepath.Join(f.Root, wf)))
	}
	if exists(filepath.Join(f.Root, ".gitlab-ci.yml")) {
		f.ConfigFiles = append(f.ConfigFiles, ToolchainFile{Path: ".gitlab-ci.yml", Detail: "gitlab ci"})
		extractCIRuns(f, readFile(filepath.Join(f.Root, ".gitlab-ci.yml")))
	}
}

// extractCIRuns pulls shell-ish lines out of `run:` blocks. This is best
// effort: CI files exist to hint at canonical commands, not to be parsed.
func extractCIRuns(f *Facts, content string) {
	if content == "" {
		return
	}
	lines := strings.Split(content, "\n")
	for i, raw := range lines {
		m := ciRunRe.FindStringSubmatch(raw)
		if m == nil {
			continue
		}
		frag := strings.TrimSpace(m[1])
		if isYAMLBlockScalar(frag) {
			// YAML block scalars end when indentation returns to the level of the
			// `run:` key. Merely checking for "some indentation" leaks the next
			// step's `uses:`, `with:` and other YAML fields into CI commands.
			runIndent := leadingIndent(raw)
			blockIndent := -1
			for j := i + 1; j < len(lines); j++ {
				next := lines[j]
				if strings.TrimSpace(next) == "" {
					continue
				}
				indent := leadingIndent(next)
				if indent <= runIndent {
					break
				}
				if blockIndent < 0 {
					blockIndent = indent
				}
				if indent < blockIndent {
					break
				}
				cmd := strings.TrimRight(next[blockIndent:], " \t\r")
				if strings.TrimSpace(cmd) != "" {
					f.CICommands = addUnique(f.CICommands, cmd)
				}
			}
			continue
		}
		f.CICommands = addUnique(f.CICommands, frag)
	}
}
