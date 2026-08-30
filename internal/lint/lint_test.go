package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/youwei792/agentsmd/internal/analyze"
)

func writeRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	return root
}

func runLint(t *testing.T, root string) *Report {
	t.Helper()
	facts, err := analyze.Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	return Run(root, facts)
}

func TestMissingAgentsMdScoresLow(t *testing.T) {
	root := writeRoot(t, map[string]string{
		"package.json": `{"name":"x","scripts":{"test":"vitest"}}`,
	})
	rep := runLint(t, root)
	if rep.Score > 40 {
		t.Errorf("expected low score without AGENTS.md, got %d", rep.Score)
	}
	found := false
	for _, f := range rep.Findings {
		if f.Rule == "AGENTS-MISSING" {
			found = true
		}
	}
	if !found {
		t.Error("expected AGENTS-MISSING finding")
	}
}

func TestPlaceholdersAndVagueRulesDetected(t *testing.T) {
	doc := "# AGENTS.md\n\n## Commands\n\n```bash\n```\n\n## Style\n\n- Write good code\n- Be nice\n- Do your best\n\n<!-- TODO: fill -->\n"
	root := writeRoot(t, map[string]string{"AGENTS.md": doc})
	rep := runLint(t, root)
	rules := map[string]bool{}
	for _, f := range rep.Findings {
		rules[f.Rule] = true
	}
	if !rules["PLACEHOLDERS"] {
		t.Error("expected PLACEHOLDERS finding")
	}
	if !rules["VAGUE-RULES"] {
		t.Error("expected VAGUE-RULES finding")
	}
}

func TestHealthyDocScoresWell(t *testing.T) {
	doc := "# AGENTS.md\n\n## Commands\n\n### Testing\n\n```bash\nmake test\n```\n\n### Build\n\n```bash\nmake build\n```\n\n## Code style\n\nUse gofmt; keep functions small and tested.\n"
	root := writeRoot(t, map[string]string{
		"AGENTS.md": doc,
		"Makefile":  "test:\n\techo t\n\nbuild:\n\techo b\n",
	})
	rep := Run(root, nil)
	if rep.Score < 80 {
		t.Errorf("expected healthy score, got %d (%+v)", rep.Score, rep.Findings)
	}
}

func TestCLAUDEWithoutImportFlagged(t *testing.T) {
	root := writeRoot(t, map[string]string{
		"AGENTS.md":  "# AGENTS.md\n\ncontent\n",
		"CLAUDE.md":  "# CLAUDE.md\n\nsome stale duplicated content\n",
	})
	rep := runLint(t, root)
	found := false
	for _, f := range rep.Findings {
		if f.Rule == "CLAUDE-NOT-SYNCED" {
			found = true
		}
	}
	if !found {
		t.Error("expected CLAUDE-NOT-SYNCED finding")
	}
}

func TestGradeBoundaries(t *testing.T) {
	cases := []struct{ score int; want string }{
		{95, "A"}, {85, "B"}, {70, "C"}, {50, "D"}, {10, "F"},
	}
	for _, c := range cases {
		if got := Grade(c.score); got != c.want {
			t.Errorf("Grade(%d) = %q, want %q", c.score, got, c.want)
		}
	}
}

func TestPackageManagerMismatch(t *testing.T) {
	doc := "# AGENTS.md\n\n## Commands\n\n```bash\nyarn install\nyarn test\n```\n"
	root := writeRoot(t, map[string]string{
		"AGENTS.md":    doc,
		"package.json": `{"name":"x"}`,
		"package-lock.json": "{}",
	})
	rep := runLint(t, root)
	found := false
	for _, f := range rep.Findings {
		if f.Rule == "PM-MISMATCH" && strings.Contains(f.Message, "yarn") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected PM-MISMATCH, got %+v", rep.Findings)
	}
}
