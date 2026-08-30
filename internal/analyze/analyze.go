package analyze

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// exists reports whether path exists (file or dir).
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isDir reports whether path exists and is a directory.
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// readFile reads a small file, returning "" on any error.
func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// listDir returns sorted directory entry names of dir ("" on error).
func listDir(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// addUnique appends v to s if not already present.
func addUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

// findFiles walks root (max depth 3, skipping heavy dirs) and returns
// relative paths matching any of the given base names (case-insensitive).
// Patterns containing "*" are matched as globs against the base name.
func findFiles(root string, names ...string) []string {
	exact := make(map[string]bool)
	var globs []string
	for _, n := range names {
		if strings.Contains(n, "*") {
			globs = append(globs, strings.ToLower(n))
		} else {
			exact[strings.ToLower(n)] = true
		}
	}
	match := func(base string) bool {
		lb := strings.ToLower(base)
		if exact[lb] {
			return true
		}
		for _, g := range globs {
			if ok, _ := filepath.Match(g, lb); ok {
				return true
			}
		}
		return false
	}
	var out []string
	depth := 0
	skip := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "dist": true,
		"build": true, "target": true, ".venv": true, "venv": true,
		"__pycache__": true, ".tox": true, ".mypy_cache": true,
		".pytest_cache": true, ".ruff_cache": true, ".next": true,
		".turbo": true, "coverage": true, ".nuxt": true, ".output": true,
	}
	var walk func(dir string, d int)
	walk = func(dir string, d int) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			rel := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if !skip[e.Name()] && d < 3 {
					walk(rel, d+1)
				}
				continue
			}
			if match(e.Name()) {
				relToRoot, err := filepath.Rel(root, rel)
				if err == nil {
					out = append(out, relToRoot)
				}
			}
		}
	}
	walk(root, depth)
	sort.Strings(out)
	return out
}

// FindFiles is the exported form of findFiles (used by validate).
func FindFiles(root string, names ...string) []string { return findFiles(root, names...) }

// ListTopDirs returns sorted top-level directory names of root.
func ListTopDirs(root string) []string {
	var out []string
	for _, e := range listDir(root) {
		if isDir(filepath.Join(root, e)) {
			out = append(out, e)
		}
	}
	return out
}

// Analyze walks the repository at root and populates Facts.
func Analyze(root string) (*Facts, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	f := &Facts{Root: abs}

	analyzeNode(f)
	analyzePython(f)
	analyzeGo(f)
	analyzeRust(f)
	analyzeMake(f)
	analyzeDocker(f)
	analyzeCI(f)
	analyzeLinters(f)
	analyzeAgentFiles(f)

	if len(f.Manifests) == 0 && len(f.Languages) == 0 {
		f.Warnings = append(f.Warnings, "no known manifests found; is this the right directory?")
	}
	return f, nil
}

// analyzeDocker detects Docker files.
func analyzeDocker(f *Facts) {
	candidates := []string{"Dockerfile", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml", ".dockerignore"}
	var found []string
	for _, c := range candidates {
		if exists(filepath.Join(f.Root, c)) {
			found = append(found, c)
		}
	}
	// Also nested Dockerfiles (docker/*/Dockerfile patterns are common).
	for _, p := range findFiles(f.Root, "Dockerfile") {
		if !contains(found, p) {
			found = append(found, p)
		}
	}
	if len(found) > 0 {
		f.Docker = true
		f.ConfigFiles = append(f.ConfigFiles, ToolchainFile{Path: found[0], Detail: "docker"})
		for _, p := range found[1:] {
			f.ConfigFiles = append(f.ConfigFiles, ToolchainFile{Path: p, Detail: "docker"})
		}
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
