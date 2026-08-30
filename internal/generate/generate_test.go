package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/youwei792/agentsmd/internal/analyze"
)

func analyzeRepo(t *testing.T, root string) *analyze.Facts {
	t.Helper()
	f, err := analyze.Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestGenerateGroundsCommands(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "package.json"), []byte(`{
		"name": "app",
		"scripts": {"test": "vitest run", "build": "vite build", "lint": "eslint ."}
	}`), 0o644)

	f := analyzeRepo(t, root)
	res := Build(f, Options{})
	c := res.Content
	if !strings.Contains(c, "```bash") {
		t.Error("expected a bash block")
	}
	if !strings.Contains(c, "npm run test") || !strings.Contains(c, "npm run build") {
		t.Error("expected grounded commands, got:\n" + c)
	}
	if strings.Contains(c, "yarn test") {
		t.Error("must not invent commands that were not detected")
	}
	if res.TODOs == 0 {
		t.Error("expected some TODO placeholders for human input")
	}
}

func TestGenerateMinimal(t *testing.T) {
	root := t.TempDir()
	f := analyzeRepo(t, root)
	res := Build(f, Options{Minimal: true})
	if strings.Contains(res.Content, "## Project overview") {
		t.Error("minimal mode should skip overview")
	}
	if !strings.Contains(res.Content, "# AGENTS.md") {
		t.Error("minimal mode still needs the header")
	}
}
