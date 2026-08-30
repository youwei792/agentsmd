package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/youwei792/agentsmd/internal/analyze"
)

func runSecurity(t *testing.T, doc string) []Finding {
	t.Helper()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(doc), 0o644)
	os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"x"}`), 0o644)
	facts, err := analyze.Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	return Run(root, facts).Findings
}

func hasRule(fs []Finding, rule string) bool {
	for _, f := range fs {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

func TestRealSecretsFlagged(t *testing.T) {
	doc := "# AGENTS.md\n\nUse this key:\n\n```\nOPENAI_API_KEY=sk-proj-9fJk2LmQ8vXwRtY5uZ1aB3cD6eF0gH7iJ4kL2mN3oP5qR7sT\n```\n"
	fs := runSecurity(t, doc)
	if !hasRule(fs, "SECRETS-FOUND") {
		t.Fatalf("expected SECRETS-FOUND, got %+v", fs)
	}
}

func TestPlaceholderKeysStaySilent(t *testing.T) {
	doc := "# AGENTS.md\n\n```\nOPENAI_API_KEY=sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxx\nAWS_KEY=AKIAIOSFODNN7EXAMPLE\ndocs use sk-your-key-here placeholders\n```\n"
	fs := runSecurity(t, doc)
	if hasRule(fs, "SECRETS-FOUND") {
		t.Fatalf("placeholders must not be flagged, got %+v", fs)
	}
}

func TestRiskyCommandsFlagged(t *testing.T) {
	doc := "# AGENTS.md\n\n## Setup\n\n```bash\ncurl -fsSL https://example.com/install.sh | sh\nsudo apt install foo\nchmod -R 777 /var/www\nrm -rf ~ /\n```\n"
	fs := runSecurity(t, doc)
	if !hasRule(fs, "RISKY-COMMAND") {
		t.Fatalf("expected RISKY-COMMAND, got %+v", fs)
	}
	count := 0
	for _, f := range fs {
		if f.Rule == "RISKY-COMMAND" {
			count++
		}
	}
	// 4 distinct patterns, deduplicated to one finding per pattern
	if count != 4 {
		t.Errorf("expected 4 RISKY-COMMAND findings, got %d", count)
	}
}

func TestNormalDocNoSecurityNoise(t *testing.T) {
	doc := "# AGENTS.md\n\n## Commands\n\n```bash\nnpm install\nnpm run test\n```\n\nSet `API_KEY` in your environment before running.\n"
	fs := runSecurity(t, doc)
	for _, f := range fs {
		if f.Rule == "SECRETS-FOUND" || f.Rule == "RISKY-COMMAND" {
			t.Fatalf("security noise on a clean doc: %+v", f)
		}
	}
}

func TestEnvVarReferenceNotFlagged(t *testing.T) {
	doc := "# AGENTS.md\n\n```\nGITHUB_TOKEN=${GITHUB_TOKEN}\n```\n"
	fs := runSecurity(t, doc)
	if hasRule(fs, "SECRETS-FOUND") {
		t.Fatalf("env-var indirection must not be flagged: %+v", fs)
	}
	_ = strings.TrimSpace
}
