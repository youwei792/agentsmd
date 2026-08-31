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

func TestGenerateDoesNotTurnWorkflowMetadataIntoShell(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0o755)
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.24\n"), 0o644)
	os.WriteFile(filepath.Join(root, ".github", "workflows", "ci.yml"), []byte(`name: CI
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: |
          go test ./...
      - name: Release
        uses: softprops/action-gh-release@v2
        with:
          generate_release_notes: true
`), 0o644)

	res := Build(analyzeRepo(t, root), Options{})
	if !strings.Contains(res.Content, "go test ./...") {
		t.Fatalf("expected grounded CI command, got:\n%s", res.Content)
	}
	for _, forbidden := range []string{"- name: Release", "uses: softprops", "generate_release_notes:"} {
		if strings.Contains(res.Content, forbidden) {
			t.Errorf("workflow metadata %q leaked into generated shell block", forbidden)
		}
	}
}
