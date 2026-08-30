// Package mdutil extracts machine-checkable facts from agent-instruction
// markdown: fenced shell commands and backticked file/command references.
package mdutil

import (
	"path/filepath"
	"regexp"
	"strings"
)

// CodeBlock is a fenced code block from a markdown document.
type CodeBlock struct {
	Info     string // fence info string (e.g. "bash")
	StartLn  int    // 1-based line number of the opening fence
	Lines    []string
	RawLines []string // original lines including any "$ " prompt prefix
}

// ShellCommand is a candidate command line found in a document.
type ShellCommand struct {
	Cmd    string // cleaned command text
	File   string // document path (as given to Parse)
	Line   int    // 1-based line number
	Source string // "fence" or "inline"
	Dir    string // active `cd` target within the same fenced block, if any
}

// PathRef is a backticked token that looks like a file path.
type PathRef struct {
	Path string
	File string
	Line int
}

var fenceRe = regexp.MustCompile("^\\s*(`{3,}|~{3,})(.*)$")

// shellLangs are fence info strings treated as shell.
var shellLangs = map[string]bool{
	"bash": true, "sh": true, "shell": true, "zsh": true, "fish": true,
	"console": true, "terminal": true, "powershell": true, "ps1": true,
	"bat": true, "cmd": true, "": true,
}

// KnownRunners are first words we know how to validate.
var KnownRunners = map[string]bool{
	"npm": true, "npx": true, "pnpm": true, "pnpx": true, "yarn": true,
	"bun": true, "bunx": true, "node": true, "deno": true, "tsx": true,
	"ts-node": true, "python": true, "python3": true, "pip": true,
	"pip3": true, "pipenv": true, "uv": true, "uvx": true, "poetry": true,
	"pytest": true, "ruff": true, "black": true, "mypy": true,
	"flake8": true, "pylint": true, "isort": true, "tox": true, "nox": true,
	"go": true, "cargo": true, "rustc": true, "rustup": true,
	"make": true, "just": true, "cmake": true, "meson": true, "ninja": true,
	"docker": true, "docker-compose": true, "git": true, "gh": true,
	"mvn": true, "gradle": true, "./gradlew": true, "./mvnw": true,
	"dotnet": true, "mix": true, "rake": true, "bundle": true,
	"composer": true, "php": true, "ruby": true, "java": true,
	"terraform": true, "tofu": true, "kubectl": true, "helm": true,
	"vitest": true, "jest": true, "eslint": true, "prettier": true,
	"tsc": true, "biome": true, "golangci-lint": true, "sqlx": true,
	"turbo": true, "nx": true,
}

// Parse extracts code blocks, shell commands and path references from a
// markdown document. Only confident extractions are returned.
func Parse(file, content string) ([]CodeBlock, []ShellCommand, []PathRef) {
	lines := strings.Split(content, "\n")
	var blocks []CodeBlock
	var cmds []ShellCommand
	var paths []PathRef

	inFence := false
	fence := ""
	cur := CodeBlock{}

	flushBlock := func() {
		if len(cur.Lines) > 0 {
			blocks = append(blocks, cur)
		}
		cur = CodeBlock{}
	}

	for i, raw := range lines {
		ln := i + 1
		if m := fenceRe.FindStringSubmatch(raw); m != nil {
			tick := m[1]
			if !inFence {
				inFence = true
				fence = strings.Repeat(string(tick[0]), len(tick))
				cur = CodeBlock{Info: strings.TrimSpace(m[2]), StartLn: ln}
			} else if strings.HasPrefix(tick, fence) {
				inFence = false
				flushBlock()
			}
			continue
		}
		if inFence {
			cur.RawLines = append(cur.RawLines, raw)
			cur.Lines = append(cur.Lines, raw)
			continue
		}

		// Outside fences: inline backtick references.
		for _, tok := range inlineBackticks(raw) {
			tok = cleanInlineToken(tok)
			if tok == "" {
				continue
			}
			if isCandidateCommand(tok) {
				cmds = append(cmds, ShellCommand{Cmd: tok, File: file, Line: ln, Source: "inline"})
			} else if looksLikePath(tok) {
				paths = append(paths, PathRef{Path: tok, File: file, Line: ln})
			}
		}
	}
	if inFence {
		flushBlock() // unterminated fence; still return what we have
	}

	// Extract commands from shell-ish fenced blocks.
	for _, b := range blocks {
		info := ""
		if fields := strings.Fields(b.Info); len(fields) > 0 {
			info = strings.ToLower(fields[0])
		}
		if !shellLangs[info] {
			continue
		}
		// Track `cd X` state across lines of the same block so that
		//   cd ui-tui
		//   npm run dev
		// resolves "npm run dev" against ui-tui/, not the repo root.
		curDir := ""
		for j, raw := range b.RawLines {
			cmd, ok := cleanCommand(raw, info)
			if !ok {
				continue
			}
			if t := cdTarget(cmd); t != "" {
				if t == "-" || t == ".." || strings.HasPrefix(t, "/") || strings.HasPrefix(t, "~") {
					curDir = ""
				} else {
					curDir = filepath.ToSlash(filepath.Join(filepath.ToSlash(curDir), t))
				}
				continue // cd lines themselves need no validation
			}
			cmds = append(cmds, ShellCommand{Cmd: cmd, File: file, Line: b.StartLn + 1 + j, Source: "fence", Dir: curDir})
		}
	}
	return blocks, cmds, paths
}

// cdTarget extracts the directory from a pure `cd X` command, or "" when
// the line is something else (cd in a pipeline is left for checkSegment).
func cdTarget(cmd string) string {
	f := strings.Fields(cmd)
	if len(f) == 2 && f[0] == "cd" {
		return strings.Trim(f[1], `"'`)
	}
	return ""
}

// cleanCommand strips prompts/comments and decides if a fenced line is a
// command. In "console" blocks only prompted lines count; in shell blocks
// every non-comment line is a candidate.
func cleanCommand(raw, info string) (string, bool) {
	line := strings.TrimRight(raw, " \t\r")
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false
	}
	// Strip common prompts.
	prompted := false
	for _, p := range []string{"$ ", "> ", "# ", "% ", "PS C:\\> ", "PS> ", "❯ "} {
		if strings.HasPrefix(trimmed, p) {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, p))
			prompted = true
			break
		}
	}
	if strings.HasPrefix(trimmed, "$") && len(trimmed) > 1 && !prompted {
		trimmed = strings.TrimSpace(trimmed[1:])
		prompted = true
	}
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	if info == "console" || info == "terminal" {
		if !prompted {
			return "", false // likely output
		}
	}
	// Lines that are clearly prose-wrapped output.
	if strings.HasPrefix(trimmed, "at ") || strings.HasPrefix(trimmed, "Traceback") {
		return "", false
	}
	return trimmed, true
}

var backtickRe = regexp.MustCompile("`([^`\n]+)`")

func inlineBackticks(line string) []string {
	var out []string
	for _, m := range backtickRe.FindAllStringSubmatch(line, -1) {
		out = append(out, strings.TrimSpace(m[1]))
	}
	return out
}

// isCandidateCommand reports whether a backticked token looks like a command
// we know how to validate.
func isCandidateCommand(tok string) bool {
	if strings.ContainsAny(tok, "\n") || strings.Contains(tok, "``") {
		return false
	}
	first := firstWord(tok)
	if first == "" {
		return false
	}
	if KnownRunners[first] {
		return strings.Contains(tok, " ")
	}
	// ./script or path-to-binary style
	if strings.HasPrefix(first, "./") && len(first) > 2 {
		return true
	}
	return false
}

var pathExtRe = regexp.MustCompile(`\.(md|txt|json|ya?ml|toml|ini|cfg|ts|tsx|js|jsx|mjs|cjs|go|py|rs|rb|java|kt|swift|c|h|cpp|hpp|cs|php|sh|bash|zsh|fish|sql|html|css|scss|vue|svelte|proto|tf|hcl|env|lock|sum|mod)$`)
var envFileRe = regexp.MustCompile(`^\.env(\.\w+)?$`)
var placeholderRe = regexp.MustCompile(`<[\w][\w ./-]*>`)

// conceptualFileNames are agent-instruction files that docs mention
// conceptually ("the CLAUDE.md of this project") far more often than they
// reference as an existing path. A missing one is not a finding.
var conceptualFileNames = map[string]bool{
	"AGENTS.md": true, "CLAUDE.md": true, "GEMINI.md": true,
	".cursorrules": true, "copilot-instructions.md": true,
}

// cleanInlineToken normalizes an inline backtick token. Returns "" when the
// token is punctuation noise (markdown tables/lists leave marks inside
// backticks) or a bare file extension (".ts" in a prose table is a type).
func cleanInlineToken(tok string) string {
	tok = strings.TrimRight(tok, ")],.;:")
	tok = strings.TrimLeft(tok, "([")
	tok = strings.TrimSpace(tok)
	if len(tok) >= 2 && (tok[0] == tok[len(tok)-1]) && (tok[0] == '"' || tok[0] == '\'') {
		tok = tok[1 : len(tok)-1]
	}
	if strings.Trim(tok, ".") == "" {
		return ""
	}
	if strings.HasPrefix(tok, ".") && !strings.ContainsAny(tok, "/\\ ") && len(tok) <= 6 {
		return "" // bare extension like ".ts"
	}
	return tok
}

// looksLikePath reports whether a backticked token is probably a file path.
func looksLikePath(tok string) bool {
	if tok == "" || strings.Contains(tok, " ") {
		return false
	}
	if strings.HasPrefix(tok, "http://") || strings.HasPrefix(tok, "https://") {
		return false
	}
	if strings.HasPrefix(tok, "@") {
		return false
	}
	if placeholderRe.MatchString(tok) {
		return false // template reference, not a literal path
	}
	if strings.Contains(tok, "/") {
		return true
	}
	return pathExtRe.MatchString(tok) || envFileRe.MatchString(tok)
}

// firstWord returns the first whitespace-separated token.
func firstWord(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// SplitPipeline breaks a command into simple segments on &&, ||, ; and |,
// stripping env assignments and cd wrappers. Returns segments in order.
func SplitPipeline(cmd string) []string {
	// Shell-aware split that respects quotes.
	var segs []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if quote != 0 {
			cur.WriteByte(c)
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
			cur.WriteByte(c)
		case '&', '|', ';':
			if c == '&' && i+1 < len(cmd) && cmd[i+1] == '&' {
				segs = append(segs, cur.String())
				cur.Reset()
				i++
				continue
			}
			if c == '|' && i+1 < len(cmd) && cmd[i+1] == '|' {
				segs = append(segs, cur.String())
				cur.Reset()
				i++
				continue
			}
			if c == ';' || c == '|' {
				segs = append(segs, cur.String())
				cur.Reset()
				continue
			}
			cur.WriteByte(c)
		default:
			cur.WriteByte(c)
		}
	}
	segs = append(segs, cur.String())

	out := make([]string, 0, len(segs))
	for _, s := range segs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		// Strip env assignments at the front.
		for {
			f := strings.Fields(s)
			if len(f) > 1 && strings.Contains(f[0], "=") && !strings.HasPrefix(f[0], "./") && isEnvAssign(f[0]) {
				s = strings.TrimSpace(strings.TrimPrefix(s, f[0]))
				continue
			}
			break
		}
		out = append(out, s)
	}
	return out
}

func isEnvAssign(tok string) bool {
	eq := strings.Index(tok, "=")
	if eq <= 0 {
		return false
	}
	for i := 0; i < eq; i++ {
		c := tok[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}
