package analyze

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyzeNodeProject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{
		"name": "app",
		"scripts": {"test": "vitest run", "build": "vite build", "lint": "eslint ."},
		"dependencies": {"express": "^4.0.0"},
		"devDependencies": {"vitest": "^1.0.0", "eslint": "^9.0.0"}
	}`)
	writeFile(t, root, "pnpm-lock.yaml", "")

	f, err := Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Has("TypeScript/JavaScript") {
		t.Errorf("expected TS/JS language, got %v", f.Languages)
	}
	if pm := f.PackageManager(); pm != "pnpm" {
		t.Errorf("expected pnpm, got %q", pm)
	}
	if f.Script("test") == nil {
		t.Error("expected a test script")
	}
	if f.Script("test").Cmdline != "pnpm test" {
		t.Errorf("pnpm repo must generate pnpm commands, got %q", f.Script("test").Cmdline)
	}
	foundExpress := false
	for _, fw := range f.Frameworks {
		if fw.Name == "Express" {
			foundExpress = true
		}
	}
	if !foundExpress {
		t.Error("expected Express framework")
	}
	if !contains(f.Linters, "eslint") {
		t.Errorf("expected eslint linter, got %v", f.Linters)
	}
	if f.GetAgentFile("agents") != nil {
		t.Error("expected no AGENTS.md")
	}
}

func TestAnalyzePythonUV(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pyproject.toml", `
[project]
name = "demo"
requires-python = ">=3.11"
dependencies = ["fastapi>=0.110"]

[tool.pytest.ini_options]
testpaths = ["tests"]
`)
	writeFile(t, root, "uv.lock", "")

	f, err := Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Has("Python") {
		t.Errorf("expected Python, got %v", f.Languages)
	}
	if pm := f.PackageManager(); pm != "uv" {
		t.Errorf("expected uv, got %q", pm)
	}
	if !contains(f.TestFrameworks, "pytest") {
		t.Errorf("expected pytest, got %v", f.TestFrameworks)
	}
	if s := f.Script("install"); s == nil || s.Cmdline != "uv sync" {
		t.Errorf("expected uv sync install script, got %+v", s)
	}
}

func TestAnalyzeGoProject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/app

go 1.24

require github.com/gin-gonic/gin v1.10.0
`)
	writeFile(t, root, "Makefile", `build:
	go build ./...

test:
	go test ./...
`)

	f, err := Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Has("Go") {
		t.Errorf("expected Go, got %v", f.Languages)
	}
	foundGin := false
	for _, fw := range f.Frameworks {
		if fw.Name == "Gin" {
			foundGin = true
		}
	}
	if !foundGin {
		t.Error("expected Gin framework")
	}
	var hasMakeTest bool
	for _, s := range f.Scripts {
		if s.Cmdline == "make test" && s.Purpose == "test" {
			hasMakeTest = true
		}
	}
	if !hasMakeTest {
		t.Errorf("expected make test script, got %+v", f.Scripts)
	}
}

func TestAnalyzeRustWorkspace(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "Cargo.toml", `
[package]
name = "demo"
edition = "2021"

[workspace]
members = ["crates/core"]
`)
	writeFile(t, root, "src/main.rs", "fn main() {}\n")

	f, err := Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Has("Rust") {
		t.Errorf("expected Rust, got %v", f.Languages)
	}
	if f.Monorepo == nil || f.Monorepo.Kind != "cargo-workspace" {
		t.Errorf("expected cargo workspace, got %+v", f.Monorepo)
	}
	if s := f.Script("test"); s == nil || s.Cmdline != "cargo test" {
		t.Errorf("expected cargo test, got %+v", s)
	}
}

func TestAnalyzeAgentFilesAndSync(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "# AGENTS.md\n\ninstructions\n")
	writeFile(t, root, "CLAUDE.md", "@AGENTS.md\n")

	f, err := Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	agents := f.GetAgentFile("agents")
	if agents == nil || agents.Path != "AGENTS.md" {
		t.Fatalf("expected AGENTS.md, got %+v", agents)
	}
	claude := f.GetAgentFile("claude")
	if claude == nil {
		t.Fatal("expected CLAUDE.md detected")
	}
	if !claude.HasRef {
		t.Error("expected CLAUDE.md to report HasRef")
	}
}

func TestScriptCommandsMatchPackageManager(t *testing.T) {
	// Generated cmdlines must match the lockfile, or the repo's own lint
	// (PM-MISMATCH) would flag the generated file.
	cases := []struct{ pm, lock, want string }{
		{"pnpm", "pnpm-lock.yaml", "pnpm test"},
		{"yarn", "yarn.lock", "yarn test"},
		{"bun", "bun.lock", "bun run test"},
		{"npm", "package-lock.json", "npm run test"},
	}
	for _, c := range cases {
		root := t.TempDir()
		writeFile(t, root, "package.json", `{"name":"app","scripts":{"test":"vitest run"}}`)
		writeFile(t, root, c.lock, "")
		f, err := Analyze(root)
		if err != nil {
			t.Fatal(err)
		}
		if got := f.Script("test").Cmdline; got != c.want {
			t.Errorf("%s repo: generated %q, want %q", c.pm, got, c.want)
		}
	}
}
