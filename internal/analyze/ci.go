package analyze

import (
	"path/filepath"
	"regexp"
	"strings"
)

var ciRunRe = regexp.MustCompile(`(?m)^\s*(?:-\s+)?run:\s*(\|?.*)$`)

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
		if strings.HasPrefix(frag, "|") || frag == ">" || frag == "" {
			// Multiline block: take following indented lines.
			for j := i + 1; j < len(lines); j++ {
				next := lines[j]
				if strings.TrimSpace(next) == "" {
					continue
				}
				if strings.HasPrefix(next, " ") || strings.HasPrefix(next, "\t") {
					f.CICommands = addUnique(f.CICommands, strings.TrimSpace(next))
					continue
				}
				break
			}
			continue
		}
		f.CICommands = addUnique(f.CICommands, frag)
	}
}
