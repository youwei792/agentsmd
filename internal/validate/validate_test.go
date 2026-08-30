package validate

import (
	"os"
	"path/filepath"
	"strings"
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

// ---- regression tests from real-world repo validation (v0.1.1) ----

func TestLeadingSlashIsRepoRelative(t *testing.T) {
	root, facts := fixtureRepo(t)
	eng := NewEngine(root, facts)
	// frp documents "Mock servers in /test/e2e/mock/"
	write := filepath.Join(root, "test", "e2e")
	os.MkdirAll(write, 0o755)
	if out := eng.CheckDocument("AGENTS.md", "See `/test/e2e/` for e2e.\n"); len(out) != 0 {
		t.Fatalf("leading slash should be repo-relative, got %+v", out)
	}
}

func TestBuildArtifactBinaryTolerated(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/x/ollama\n\ngo 1.24\n"), 0o644)
	facts, _ := analyze.Analyze(root)
	eng := NewEngine(root, facts)
	// ollama documents "cmake --build build && ./ollama serve" — the binary
	// is the build product.
	if out := eng.CheckDocument("AGENTS.md", "```sh\ngo build .\n./ollama serve\n```\n"); len(out) != 0 {
		t.Fatalf("build-artifact binary should be tolerated, got %+v", out)
	}
}

func TestCdStateTrackedWithinBlock(t *testing.T) {
	root, facts := fixtureRepo(t)
	os.MkdirAll(filepath.Join(root, "ui-tui"), 0o755)
	os.WriteFile(filepath.Join(root, "ui-tui", "package.json"),
		[]byte(`{"name":"ui","scripts":{"dev":"vite"}}`), 0o644)
	eng := NewEngine(root, facts)
	doc := "```bash\ncd ui-tui\nnpm install\nnpm run dev\n```\n"
	if out := eng.CheckDocument("AGENTS.md", doc); len(out) != 0 {
		t.Fatalf("cd state should apply to later lines in the block, got %+v", out)
	}
	// Root scripts are still checked for commands without a cd.
	if out := eng.CheckDocument("AGENTS.md", "```bash\nnpm run dev\n```\n"); len(out) == 0 {
		t.Fatal("without cd, root package.json should be used")
	}
}

func TestHiddenRuntimeDirRefsSkipped(t *testing.T) {
	root, facts := fixtureRepo(t)
	eng := NewEngine(root, facts)
	if out := eng.CheckDocument("AGENTS.md", "Plugins live in `./.hermes/plugins/` too.\n"); len(out) != 0 {
		t.Fatalf("hidden runtime dir refs should be skipped, got %+v", out)
	}
}

func TestInlineBareDotDirSkipped(t *testing.T) {
	root, facts := fixtureRepo(t)
	eng := NewEngine(root, facts)
	if out := eng.CheckDocument("AGENTS.md", "Use `scrapeTimeout` from `./lib` to set timeouts.\n"); len(out) != 0 {
		t.Fatalf("inline ./lib shorthand should be skipped, got %+v", out)
	}
}

func TestConceptualAgentFileMentionsSkipped(t *testing.T) {
	root, facts := fixtureRepo(t)
	eng := NewEngine(root, facts)
	out := eng.CheckDocument("AGENTS.md", "Context file (`CLAUDE.md`, etc.) not updated?\n")
	for _, f := range out {
		if strings.Contains(f.Message, "CLAUDE.md") {
			t.Fatalf("conceptual CLAUDE.md mention flagged: %+v", out)
		}
	}
}

func TestPlaceholderPathRefsSkipped(t *testing.T) {
	root, facts := fixtureRepo(t)
	eng := NewEngine(root, facts)
	if out := eng.CheckDocument("AGENTS.md", "Edit `src/specify_cli/integrations/<package_dir>/`.\n"); len(out) != 0 {
		t.Fatalf("placeholder path refs should be skipped, got %+v", out)
	}
}
