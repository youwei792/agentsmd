package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureSkill(t *testing.T, dir, frontmatter, body string) string {
	t.Helper()
	root := t.TempDir()
	full := filepath.Join(root, ".claude", "skills", dir)
	os.MkdirAll(full, 0o755)
	os.WriteFile(filepath.Join(full, "SKILL.md"), []byte("---\n"+frontmatter+"---\n\n"+body), 0o644)
	return root
}

func TestValidSkillPasses(t *testing.T) {
	root := fixtureSkill(t, "pdf-tools",
		"name: pdf-tools\ndescription: Extract text and tables from PDF files and merge or split documents.\nallowed-tools: Read, Bash\n",
		"Use `scripts/extract.py`.\n")
	os.MkdirAll(filepath.Join(root, ".claude", "skills", "pdf-tools", "scripts"), 0o755)
	os.WriteFile(filepath.Join(root, ".claude", "skills", "pdf-tools", "scripts", "extract.py"), []byte("x"), 0o644)

	reports := Run(root)
	if len(reports) != 1 || !reports[0].Valid {
		t.Fatalf("expected valid skill, got %+v", reports)
	}
	if reports[0].Name != "pdf-tools" || reports[0].Tokens == 0 {
		t.Errorf("unexpected report: %+v", reports[0])
	}
}

func TestNameMismatchAndBadFormat(t *testing.T) {
	root := fixtureSkill(t, "pdf-tools",
		"name: Bad_Name\ndescription: short\n", "body\n")
	reports := Run(root)
	if len(reports) != 1 || reports[0].Valid {
		t.Fatalf("expected invalid skill, got %+v", reports)
	}
	msgs := ""
	for _, f := range reports[0].Findings {
		msgs += f.Message + "\n"
	}
	if !strings.Contains(msgs, "lowercase") {
		t.Errorf("expected name format error, got %s", msgs)
	}
}

func TestMissingFrontmatterAndBundleRefs(t *testing.T) {
	root := t.TempDir()
	full := filepath.Join(root, "skills", "bare")
	os.MkdirAll(full, 0o755)
	os.WriteFile(filepath.Join(full, "SKILL.md"), []byte("# No frontmatter\n\nSee `missing.md`.\n"), 0o644)

	reports := Run(root)
	if len(reports) != 1 || reports[0].Valid {
		t.Fatalf("expected invalid, got %+v", reports)
	}
	msgs := ""
	for _, f := range reports[0].Findings {
		msgs += f.Message + "\n"
	}
	if !strings.Contains(msgs, "frontmatter") || !strings.Contains(msgs, "missing.md") {
		t.Errorf("expected frontmatter + bundle-ref findings, got %s", msgs)
	}
}
