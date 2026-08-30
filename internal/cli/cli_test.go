package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary compiles agentsmd once for the whole test run.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "agentsmd")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/youwei792/agentsmd")
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, stderr.String())
	}
	return bin
}

func runCLI(t *testing.T, bin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	return out.String(), code
}

func TestEndToEndDoctor(t *testing.T) {
	bin := buildBinary(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "package.json"), []byte(`{
		"name": "app",
		"scripts": {"test": "vitest run", "build": "vite build"},
		"devDependencies": {"vitest": "^1.0.0"}
	}`), 0o644)

	// No AGENTS.md yet -> doctor should fail.
	out, code := runCLI(t, bin, "doctor", root)
	if code == 0 {
		t.Errorf("expected nonzero exit without AGENTS.md, got output:\n%s", out)
	}

	// init -> sync -> doctor should pass.
	if out, code = runCLI(t, bin, "init", root); code != 0 {
		t.Fatalf("init failed:\n%s", out)
	}
	if out, code = runCLI(t, bin, "sync", root); code != 0 {
		t.Fatalf("sync failed:\n%s", out)
	}
	out, code = runCLI(t, bin, "doctor", root)
	if code != 0 {
		t.Errorf("doctor failed after init+sync:\n%s", out)
	}
	if !strings.Contains(out, "Context footprint") {
		t.Errorf("expected doctor report, got:\n%s", out)
	}

	// check with a broken command must fail.
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(
		"# AGENTS.md\n\n## Commands\n\n```bash\nnpm run does-not-exist\n```\n"), 0o644)
	out, code = runCLI(t, bin, "check", root)
	if code == 0 {
		t.Errorf("check should fail on broken script, got:\n%s", out)
	}

	// json output parses as flag.
	out, code = runCLI(t, bin, "tokens", root, "--json")
	if code != 0 || !strings.Contains(out, "\"total_tokens\"") {
		t.Errorf("tokens --json failed:\n%s", out)
	}
}

func TestVersionCommand(t *testing.T) {
	bin := buildBinary(t)
	out, code := runCLI(t, bin, "version")
	if code != 0 || !strings.Contains(out, "agentsmd") {
		t.Errorf("version failed: %q", out)
	}
}
