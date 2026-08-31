package checkcmd

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInspectIncludesNestedInstructions(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "AGENTS.md", "# Root\n")
	writeFixture(t, root, "packages/widget/package.json", `{
		"name":"widget","scripts":{"test":"node test.js"}
	}`)
	writeFixture(t, root, "packages/widget/AGENTS.md", "```bash\nnpm run missing\n```\n")
	writeFixture(t, root, "node_modules/dependency/AGENTS.md", "```bash\nnpm run ignored\n```\n")

	rep, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Documents) != 2 {
		t.Fatalf("expected root and nested documents only, got %+v", rep.Documents)
	}
	if rep.Documents[1].Path != "packages/widget/AGENTS.md" {
		t.Fatalf("nested document missing or unordered: %+v", rep.Documents)
	}
	if rep.Errors != 1 || len(rep.Documents[1].Findings) != 1 {
		t.Fatalf("broken nested command was not detected: %+v", rep)
	}
	if got := rep.Documents[1].Findings[0].File; got != "packages/widget/AGENTS.md" {
		t.Fatalf("finding should name its real document, got %q", got)
	}
}

func TestInspectRejectsInstructionSymlinkOutsideRepository(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "AGENTS.md")
	writeFixture(t, filepath.Dir(outside), filepath.Base(outside), "# private\n")
	if err := os.Symlink(outside, filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}

	rep, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Documents) != 1 || rep.Errors != 1 {
		t.Fatalf("external instruction symlink should be a check error: %+v", rep)
	}
}

func TestInspectAcceptsNestedRelativeReferences(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "services/api/AGENTS.md", "See `docs/runbook.md`.\n")
	writeFixture(t, root, "services/api/docs/runbook.md", "runbook\n")

	rep, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Errors != 0 || rep.Warnings != 0 {
		t.Fatalf("nested relative reference should pass: %+v", rep)
	}
}
