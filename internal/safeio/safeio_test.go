package safeio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileWithin(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "docs", "rules.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("docs", "rules.md"), filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	b, err := ReadFileWithin(root, "AGENTS.md")
	if err != nil || string(b) != "rules\n" {
		t.Fatalf("internal symlink should be readable: %q, %v", b, err)
	}
}

func TestReadFileWithinRejectsExternalSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private.md")
	if err := os.WriteFile(outside, []byte("private\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFileWithin(root, "AGENTS.md"); err == nil {
		t.Fatal("external symlink should be rejected")
	}
	if _, err := ReadFileWithin(root, "../private.md"); err == nil {
		t.Fatal("parent traversal should be rejected")
	}
}
