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

func TestCopyUsesAgentsBodyAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	body := "# AGENTS.md\n\nbody\n"
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(body), 0o644)

	res := Run(root, ModeCopy, []string{"claude"}, false, body)
	if !res.InSync || res.Actions[0].Status != "created" {
		t.Fatalf("expected successful copy, got %+v", res)
	}
	b, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil || !strings.Contains(string(b), body) {
		t.Fatalf("copy does not contain AGENTS.md body: %q, %v", b, err)
	}

	res = Run(root, ModeCopy, []string{"claude"}, false, body)
	if res.Actions[0].Status != "up-to-date" {
		t.Fatalf("second copy should be idempotent, got %+v", res.Actions[0])
	}
}

func TestImportRefusesSymlinkWithoutTouchingTarget(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# AGENTS.md\n"), 0o644)
	os.WriteFile(outside, []byte("do not change\n"), 0o644)
	if err := os.Symlink(outside, filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}

	res := Run(root, ModeImport, []string{"claude"}, false, "")
	if res.InSync || len(res.Actions) != 1 || res.Actions[0].Status != "error" {
		t.Fatalf("expected symlink refusal, got %+v", res)
	}
	b, err := os.ReadFile(outside)
	if err != nil || string(b) != "do not change\n" {
		t.Fatalf("external symlink target was changed: %q, %v", b, err)
	}
}

func TestInvalidModeAndToolFail(t *testing.T) {
	root := t.TempDir()
	for _, res := range []*Result{
		Run(root, Mode("unknown"), nil, false, ""),
		Run(root, ModeImport, []string{"cursor"}, false, ""),
	} {
		if res.InSync || len(res.ExitErrors) == 0 {
			t.Fatalf("invalid input should fail: %+v", res)
		}
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
