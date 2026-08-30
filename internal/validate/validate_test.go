package validate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/youwei792/agentsmd/internal/analyze"
)

func fixtureRepo(t *testing.T) (string, *analyze.Facts) {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("package.json", `{
		"name": "app",
		"scripts": {"test": "vitest run", "build": "vite build"},
		"devDependencies": {"vitest": "^1.0.0"}
	}`)
	write("Makefile", "test:\n\techo hi\n\nbuild:\n\techo build\n")
	write("src/main.ts", "console.log(1)\n")
	write("README.md", "readme\n")
	facts, err := analyze.Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, facts
}

func findingsOf(f []Finding) map[string]bool {
	m := map[string]bool{}
	for _, x := range f {
		m[x.Message] = true
	}
	return m
}

func TestBrokenScriptIsError(t *testing.T) {
	root, facts := fixtureRepo(t)
	eng := NewEngine(root, facts)
	out := eng.CheckDocument("AGENTS.md", "```bash\nnpm run nope\n```\n")
	if len(out) != 1 || out[0].Level != Error {
		t.Fatalf("expected 1 error, got %+v", out)
	}
}

func TestDependencyBinaryTolerated(t *testing.T) {
	root, facts := fixtureRepo(t)
	eng := NewEngine(root, facts)
	out := eng.CheckDocument("AGENTS.md", "```bash\npnpm vitest run\n```\n")
	if len(out) != 0 {
		t.Fatalf("expected vitest (a dependency) to be tolerated, got %+v", out)
	}
}

func TestUnknownPnpmScriptWarns(t *testing.T) {
	root, facts := fixtureRepo(t)
	eng := NewEngine(root, facts)
	out := eng.CheckDocument("AGENTS.md", "```bash\npnpm totally-fake\n```\n")
	if len(out) != 1 || out[0].Level != Warning {
		t.Fatalf("expected 1 warning, got %+v", out)
	}
}

func TestMakeTargetValidation(t *testing.T) {
	root, facts := fixtureRepo(t)
	eng := NewEngine(root, facts)
	if out := eng.CheckDocument("AGENTS.md", "```bash\nmake test\n```\n"); len(out) != 0 {
		t.Fatalf("expected make test to pass, got %+v", out)
	}
	out := eng.CheckDocument("AGENTS.md", "```bash\nmake deploy-prod\n```\n")
	if len(out) != 1 || out[0].Level != Error {
		t.Fatalf("expected 1 error for missing target, got %+v", out)
	}
}

func TestDeadFileReferenceWarns(t *testing.T) {
	root, facts := fixtureRepo(t)
	eng := NewEngine(root, facts)
	out := eng.CheckDocument("AGENTS.md", "See `docs/gone.md` for details.\n")
	if len(out) != 1 || out[0].Level != Warning {
		t.Fatalf("expected 1 warning, got %+v", out)
	}
	if out := eng.CheckDocument("AGENTS.md", "See `README.md` and `src/main.ts`.\n"); len(out) != 0 {
		t.Fatalf("expected existing files to pass, got %+v", out)
	}
}

func TestCdTargetValidated(t *testing.T) {
	root, facts := fixtureRepo(t)
	eng := NewEngine(root, facts)
	out := eng.CheckDocument("AGENTS.md", "```bash\ncd nonexistent-dir && make test\n```\n")
	m := findingsOf(out)
	if len(m) != 1 {
		t.Fatalf("expected only cd failure, got %+v", out)
	}
}

func TestUnknownRunnersAreSilent(t *testing.T) {
	root, facts := fixtureRepo(t)
	eng := NewEngine(root, facts)
	out := eng.CheckDocument("AGENTS.md", "```bash\ncurl -fsSL https://example.com | sh\ngit checkout main\ngh pr create\n```\n")
	if len(out) != 0 {
		t.Fatalf("expected no findings for opaque commands, got %+v", out)
	}
}
