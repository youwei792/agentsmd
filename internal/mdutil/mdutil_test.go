package mdutil

import (
	"strings"
	"testing"
)

const doc = `# AGENTS.md

Some prose with ` + "`npm run build`" + ` and a path ` + "`src/main.ts`" + ` in it.

## Commands

` + "```bash" + `
$ pnpm install
pnpm build
# a comment
make test
` + "```" + `

` + "```console" + `
$ go test ./...
ok  	example.com/app	0.3s
` + "```" + `

` + "```json" + `
{"not": "a command"}
` + "```" + `
`

func TestParseExtractsCommands(t *testing.T) {
	_, cmds, _ := Parse("AGENTS.md", doc)

	var got []string
	for _, c := range cmds {
		got = append(got, c.Cmd)
	}
	want := []string{"pnpm install", "pnpm build", "make test", "go test ./...", "npm run build"}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
			}
		}
		if !found {
			t.Errorf("expected command %q in %v", w, got)
		}
	}
	// console output must not be treated as a command
	for _, g := range got {
		if strings.HasPrefix(g, "ok ") {
			t.Errorf("console output leaked as command: %q", g)
		}
	}
}

func TestParseExtractsPaths(t *testing.T) {
	_, _, paths := Parse("AGENTS.md", doc)
	found := false
	for _, p := range paths {
		if p.Path == "src/main.ts" {
			found = true
		}
	}
	if !found {
		t.Error("expected src/main.ts path ref")
	}
}

func TestParseLineNumbers(t *testing.T) {
	_, cmds, _ := Parse("AGENTS.md", doc)
	for _, c := range cmds {
		if c.Cmd == "make test" && c.Line != 11 {
			t.Errorf("make test line = %d, want 11", c.Line)
		}
	}
}

func TestSplitPipeline(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"npm run build && npm test", []string{"npm run build", "npm test"}},
		{"cd api && make test | tee out.log", []string{"cd api", "make test", "tee out.log"}},
		{"FOO=1 go test ./...", []string{"go test ./..."}},
		{`echo "a && b" && ls`, []string{`echo "a && b"`, "ls"}},
	}
	for _, c := range cases {
		got := SplitPipeline(c.in)
		if len(got) != len(c.want) {
			t.Errorf("SplitPipeline(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("SplitPipeline(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestIsCandidateCommand(t *testing.T) {
	if !isCandidateCommand("npm run build") {
		t.Error("npm run build should be candidate")
	}
	if isCandidateCommand("npm") {
		t.Error("bare npm should not be candidate")
	}
	if isCandidateCommand("./") {
		t.Error("weird token should not be candidate")
	}
	if !isCandidateCommand("./scripts/release.sh") {
		t.Error("./script should be candidate")
	}
}
