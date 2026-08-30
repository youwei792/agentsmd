package analyze

import (
	"path/filepath"
	"regexp"
	"strings"
)

var makeTargetRe = regexp.MustCompile(`(?m)^([a-zA-Z0-9_][a-zA-Z0-9_.-]*):(?:[^=]|$)`)
var justRecipeRe = regexp.MustCompile(`(?m)^@?([a-zA-Z0-9_][a-zA-Z0-9_-]*)(?:\s+[^\n:]*)*:`)

func analyzeMake(f *Facts) {
	// Makefile variants.
	for _, name := range []string{"Makefile", "makefile", "GNUmakefile"} {
		path := filepath.Join(f.Root, name)
		content := readFile(path)
		if content == "" {
			continue
		}
		f.Manifests = append(f.Manifests, name)
		f.ConfigFiles = append(f.ConfigFiles, ToolchainFile{Path: name, Detail: "make targets"})
		for _, m := range makeTargetRe.FindAllStringSubmatch(content, -1) {
			target := m[1]
			if strings.HasPrefix(target, ".") {
				continue
			}
			f.Scripts = append(f.Scripts, Command{
				Name: target, Cmdline: "make " + target, Source: name,
				Purpose: classifyMakeTarget(target),
			})
		}
		break
	}

	// justfile.
	for _, name := range []string{"justfile", "Justfile", ".justfile"} {
		path := filepath.Join(f.Root, name)
		content := readFile(path)
		if content == "" {
			continue
		}
		f.Manifests = append(f.Manifests, name)
		f.ConfigFiles = append(f.ConfigFiles, ToolchainFile{Path: name, Detail: "just recipes"})
		for _, m := range justRecipeRe.FindAllStringSubmatch(content, -1) {
			recipe := m[1]
			if recipe == "" || strings.HasPrefix(recipe, "#") {
				continue
			}
			f.Scripts = append(f.Scripts, Command{
				Name: recipe, Cmdline: "just " + recipe, Source: name,
				Purpose: classifyMakeTarget(recipe),
			})
		}
		break
	}
}

func classifyMakeTarget(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "test"):
		return "test"
	case strings.Contains(n, "lint") || strings.Contains(n, "check") || strings.Contains(n, "vet"):
		return "lint"
	case strings.Contains(n, "fmt") || strings.Contains(n, "format"):
		return "format"
	case strings.Contains(n, "build") || strings.Contains(n, "compile") || n == "all":
		return "build"
	case strings.Contains(n, "run") || strings.Contains(n, "dev") || strings.Contains(n, "serve") || strings.Contains(n, "start"):
		return "dev"
	case strings.Contains(n, "install") || strings.Contains(n, "deps") || strings.Contains(n, "setup"):
		return "install"
	default:
		return "other"
	}
}
