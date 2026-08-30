package syncmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportCreatesIdempotently(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# AGENTS.md\n"), 0o644)

	res := Run(root, ModeImport, nil, false, "")
	if len(res.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %+v", res.Actions)
	}
	claude, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil || !strings.Contains(string(claude), "@AGENTS.md") {
		t.Fatalf("expected CLAUDE.md with @AGENTS.md import, got %q, err %v", claude, err)
	}

	// Second run must be a no-op.
	res2 := Run(root, ModeImport, nil, false, "")
	for _, a := range res2.Actions {
		if a.Status != "up-to-date" {
			t.Errorf("expected up-to-date on second run, got %s", a.Status)
		}
	}
}

func TestImportPreservesExistingContent(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# AGENTS.md\n"), 0o644)
	os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("# My notes\n\nkeep me\n"), 0o644)

	Run(root, ModeImport, []string{"claude"}, false, "")
	b, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	s := string(b)
	if !strings.Contains(s, "keep me") || !strings.Contains(s, "@AGENTS.md") {
		t.Errorf("expected both original content and import, got %q", s)
	}
}

func TestCheckOnlyDetectsStale(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# AGENTS.md\n"), 0o644)

	res := Run(root, ModeImport, nil, true, "")
	if res.InSync {
		t.Error("expected not-in-sync without CLAUDE.md")
	}
	for _, a := range res.Actions {
		if a.Status != "would-create" {
			t.Errorf("expected would-create, got %s", a.Status)
		}
	}
	// Nothing should have been written.
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err == nil {
		t.Error("check-only must not write files")
	}
}

func TestCopyRefusesUnmanagedFile(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# AGENTS.md\n\nbody\n"), 0o644)
	os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("human content\n"), 0o644)

	res := Run(root, ModeCopy, []string{"claude"}, false, "# AGENTS.md\n\nbody\n")
	if res.Actions[0].Status != "error" {
		t.Errorf("expected refusal, got %+v", res.Actions[0])
	}
	b, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if string(b) != "human content\n" {
		t.Error("original file must be untouched")
	}
}

func TestSymlinkMode(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# AGENTS.md\n"), 0o644)

	Run(root, ModeSymlink, []string{"claude"}, false, "")
	dest, err := os.Readlink(filepath.Join(root, "CLAUDE.md"))
	if err != nil || dest != "AGENTS.md" {
		t.Errorf("expected symlink, got %q err %v", dest, err)
	}
}

func TestImportInsertsAfterComments(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# AGENTS.md\n"), 0o644)
	os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("<!-- tool header -->\n\nreal content\n"), 0o644)

	Run(root, ModeImport, []string{"claude"}, false, "")
	b, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	s := string(b)
	if !strings.Contains(s, "real content") || !strings.Contains(s, "@AGENTS.md") {
		t.Errorf("unexpected result: %q", s)
	}
}
