// Package validate checks that commands and file paths mentioned in agent
// documentation actually exist in the repository. It is conservative by
// design: when unsure, it says nothing.
package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/youwei792/agentsmd/internal/analyze"
	"github.com/youwei792/agentsmd/internal/mdutil"
)

// Level of a finding.
type Level string

const (
	Error   Level = "error"
	Warning Level = "warning"
	Info    Level = "info"
)

// Finding is one validation result.
type Finding struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Level   Level  `json:"level"`
	Command string `json:"command,omitempty"`
	Message string `json:"message"`
}

// Engine validates commands/paths against a repository root.
type Engine struct {
	Root  string
	Facts *analyze.Facts

	// source is the origin of the command currently being checked
	// ("fence" or "inline"); inline tokens get the most conservative
	// treatment because they lack surrounding context.
	source string
	// subDir is the active `cd` target from the same fenced block; script,
	// makefile and justfile lookups prefer it over the repo root.
	subDir string

	// caches, keyed by the active subDir where relevant
	pkgCache   map[string]*pkgInfo
	makeCache  map[string]map[string]string
	justCache  map[string]map[string]string
	buildNameC *string // module/crate/package base name (nil = not loaded)
}

// NewEngine builds an Engine with lazily loaded caches.
func NewEngine(root string, facts *analyze.Facts) *Engine {
	return &Engine{
		Root:      root,
		Facts:     facts,
		pkgCache:  map[string]*pkgInfo{},
		makeCache: map[string]map[string]string{},
		justCache: map[string]map[string]string{},
	}
}

// CheckDocument validates all commands and path references in content.
func (e *Engine) CheckDocument(file, content string) []Finding {
	var out []Finding
	_, cmds, paths := mdutil.Parse(file, content)
	docDir := filepath.ToSlash(filepath.Dir(file))
	if docDir == "." || !e.escapeFree(docDir) {
		docDir = ""
	}

	for _, c := range cmds {
		e.source = c.Source
		e.subDir = docDir
		if c.Dir != "" {
			candidate := filepath.ToSlash(filepath.Join(docDir, c.Dir))
			if e.repoDirExists(candidate) {
				e.subDir = candidate
			}
		}
		out = append(out, e.checkCommandSource(file, c.Line, c.Cmd, c.Source)...)
	}
	e.source = "fence"
	e.subDir = docDir
	for _, p := range paths {
		out = append(out, e.checkPathRef(p)...)
	}
	return out
}

// placeholderRe matches template placeholders like <dir> or
// <package-directory>; commands containing them are patterns, not
// runnable commands, and must not be validated.
var placeholderRe = regexp.MustCompile(`<[\w][\w ./-]*>`)

// CheckCommand validates one command line (possibly a pipeline).
func (e *Engine) CheckCommand(file string, line int, cmd string) []Finding {
	return e.checkCommandSource(file, line, cmd, "fence")
}

func (e *Engine) checkCommandSource(file string, line int, cmd, source string) []Finding {
	if placeholderRe.MatchString(cmd) {
		return nil
	}
	var out []Finding
	for _, seg := range mdutil.SplitPipeline(cmd) {
		e.source = source
		out = append(out, e.checkSegment(file, line, cmd, seg)...)
	}
	return out
}

// buildName returns the base name of the repo's primary build target
// (Go module / crate / package name), or the directory name. A doc that
// runs "./<name>" after a build step is running a build artifact.
func (e *Engine) buildName() string {
	if e.buildNameC != nil {
		return *e.buildNameC
	}
	name := ""
	if b, err := os.ReadFile(filepath.Join(e.Root, "go.mod")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "module ") {
				parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(t, "module ")), "/")
				name = parts[len(parts)-1]
				break
			}
		}
	}
	if name == "" {
		if b, err := os.ReadFile(filepath.Join(e.Root, "Cargo.toml")); err == nil {
			s := string(b)
			if i := strings.Index(s, "name = \""); i >= 0 {
				rest := s[i+len("name = \""):]
				if k := strings.Index(rest, "\""); k >= 0 {
					name = rest[:k]
				}
			}
		}
	}
	if name == "" {
		if b, err := os.ReadFile(filepath.Join(e.Root, "package.json")); err == nil {
			var pj struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(b, &pj) == nil {
				name = pj.Name
			}
		}
	}
	if name == "" {
		name = filepath.Base(e.Root)
	}
	e.buildNameC = &name
	return name
}

func (e *Engine) checkSegment(file string, line int, full, seg string) []Finding {
	fields := shellFields(seg)
	if len(fields) == 0 {
		return nil
	}
	first := fields[0]

	// cd <dir>
	if first == "cd" {
		if len(fields) >= 2 {
			dir := strings.Trim(fields[1], `"'`)
			if dir != "-" && !e.dirExists(dir) {
				return []Finding{{File: file, Line: line, Level: Error, Command: full,
					Message: "cd target " + quote(dir) + " does not exist"}}
			}
		}
		return nil
	}
	// ./script.sh or ./gradlew or ./built-binary
	if strings.HasPrefix(first, "./") {
		rest := strings.TrimPrefix(first, "./")
		if rest == "" {
			return nil
		}
		// Inline backticked "./lib" style tokens are usually shorthand for a
		// local directory in a subdirectory's context — stay silent.
		if e.source == "inline" && !strings.Contains(rest, "/") {
			return nil
		}
		// "./ollama serve" after a build step: the binary matching the
		// module/package name is a build artifact, not a missing file.
		if !strings.Contains(rest, "/") && rest == e.buildName() {
			return nil
		}
		// Runtime discovery dirs documented with "./.foo/" rarely exist in a
		// fresh clone by design (agent state, plugin dirs).
		if strings.HasSuffix(rest, "/") && strings.HasPrefix(rest, ".") {
			return nil
		}
		if !e.relExists(first) {
			return []Finding{{File: file, Line: line, Level: Error, Command: full,
				Message: first + " does not exist in the repository"}}
		}
		return nil
	}

	switch first {
	case "npm", "pnpm":
		return e.checkNodeScript(first, fields, file, line, full)
	case "yarn", "bun":
		return e.checkYarnBun(first, fields, file, line, full)
	case "make":
		return e.checkMake(fields, file, line, full)
	case "just":
		return e.checkJust(fields, file, line, full)
	case "go":
		return e.checkGo(fields, file, line, full)
	case "cargo":
		return e.checkCargo(fields, file, line, full)
	case "pip", "pip3":
		return e.checkPip(fields, file, line, full)
	case "uv", "poetry", "pipenv":
		if len(fields) >= 2 {
			// recurse: uv run pytest / poetry run pytest / uv pip install ...
			rest := strings.TrimSpace(strings.TrimPrefix(seg, first))
			if rest != "" {
				return e.CheckCommand(file, line, rest)
			}
		}
		return nil
	case "python", "python3":
		return e.checkPython(fields, file, line, full)
	case "pytest":
		return e.checkPytest(fields, file, line, full)
	case "docker":
		return e.checkDocker(fields, file, line, full)
	case "node", "deno", "tsx", "ts-node", "ruby", "php", "java":
		// script file args
		return e.checkScriptArgFiles(fields[1:], file, line, full)
	default:
		// ruff/black/mypy/eslint/tsc/vitest/... : validate file args only
		return e.checkScriptArgFiles(fields[1:], file, line, full)
	}
}

// pnpmValueFlags consume the following token; -C/--dir point at a
// directory that should exist.
var pnpmValueFlags = map[string]bool{"-C": true, "--dir": true, "--filter": true}

func (e *Engine) checkNodeScript(runner string, fields []string, file string, line int, full string) []Finding {
	// npm run <script> | npm test | npm run test -- | pnpm -C dir <script> | pnpm dlx ...
	args := fields[1:]

	// Strip leading flags; validate -C/--dir targets on the way.
	var dirChecks []string
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		f := args[0]
		if strings.Contains(f, "=") { // --dir=packages/x
			if i := strings.Index(f, "="); i > 0 {
				val := f[i+1:]
				if f == "--dir" || strings.HasPrefix(f, "--dir=") || strings.HasPrefix(f, "-C=") {
					dirChecks = append(dirChecks, val)
				}
			}
			args = args[1:]
			continue
		}
		if pnpmValueFlags[f] && len(args) >= 2 {
			if f == "-C" || f == "--dir" {
				dirChecks = append(dirChecks, args[1])
			}
			args = args[2:]
			continue
		}
		args = args[1:]
	}
	var out []Finding
	for _, d := range dirChecks {
		if d != "." && !e.dirExists(d) {
			out = append(out, Finding{File: file, Line: line, Level: Error, Command: full,
				Message: "pnpm -C directory " + quote(d) + " does not exist"})
		}
	}
	if len(args) == 0 {
		return out
	}
	name := args[0]

	if runner == "npm" && (name == "run" || name == "exec") {
		if len(args) >= 2 && name == "run" {
			script := strings.SplitN(args[1], "=", 2)[0]
			return append(out, e.verifyScript(script, file, line, full, runner)...)
		}
		return out
	}
	if args[0] == "dlx" || args[0] == "create" || args[0] == "init" || args[0] == "install" ||
		args[0] == "i" || args[0] == "ci" || args[0] == "add" || args[0] == "publish" || args[0] == "pack" {
		return out
	}
	if name == "run" || name == "exec" {
		if len(args) >= 2 {
			out = append(out, e.verifyScriptLoose(args[1], file, line, full, runner)...)
		}
		return out
	}
	if name == "test" || name == "start" || name == "stop" || name == "restart" {
		return append(out, e.verifyScript(name, file, line, full, runner)...)
	}
	// pnpm/yarn <name> falls back to running a local binary from
	// node_modules/.bin, so only complain if it is neither a script nor a
	// dependency of the project.
	return append(out, e.verifyScriptLoose(name, file, line, full, runner)...)
}

func (e *Engine) checkYarnBun(runner string, fields []string, file string, line int, full string) []Finding {
	args := fields[1:]
	// yarn --cwd <dir> <script>
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		flag := args[0]
		if strings.Contains(flag, "=") {
			name, value, _ := strings.Cut(flag, "=")
			if name == "--cwd" || name == "--dir" || name == "-C" {
				if !e.dirExists(value) {
					return []Finding{{File: file, Line: line, Level: Error, Command: full,
						Message: runner + " --cwd directory " + quote(value) + " does not exist"}}
				}
			}
			args = args[1:]
			continue
		}
		if (flag == "--cwd" || flag == "--dir" || flag == "-C") && len(args) >= 2 {
			if !e.dirExists(args[1]) {
				return []Finding{{File: file, Line: line, Level: Error, Command: full,
					Message: runner + " --cwd directory " + quote(args[1]) + " does not exist"}}
			}
			args = args[2:]
			continue
		}
		if yarnValueFlags[flag] && len(args) >= 2 {
			args = args[2:]
			continue
		}
		args = args[1:]
	}
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "run":
		if len(args) >= 2 {
			return e.verifyScript(args[1], file, line, full, runner)
		}
		return nil
	case "add", "remove", "install", "init", "create", "dlx", "x", "publish", "global", "workspace", "workspaces", "config":
		return nil
	case "test", "start", "dev", "build":
		return e.verifyScript(args[0], file, line, full, runner)
	default:
		return e.verifyScriptLoose(args[0], file, line, full, runner)
	}
}

var yarnValueFlags = map[string]bool{
	"--cache-folder": true, "--global-folder": true, "--modules-folder": true,
	"--mutex": true, "--network-timeout": true, "--network-concurrency": true,
	"--registry": true, "--use-yarnrc": true,
}

func (e *Engine) verifyScript(name string, file string, line int, full, runner string) []Finding {
	scripts, loc := e.loadScripts()
	if loc == "" {
		if e.Facts != nil && e.Facts.Has("TypeScript/JavaScript") {
			return []Finding{{File: file, Line: line, Level: Warning, Command: full,
				Message: "no package.json found, cannot verify " + runner + " script " + quote(name)}}
		}
		return nil
	}
	if _, ok := scripts[name]; !ok {
		nearest := suggestScript(scripts, name)
		msg := "script " + quote(name) + " is not defined in " + loc
		if nearest != "" {
			msg += " (closest: " + nearest + ")"
		}
		return []Finding{{File: file, Line: line, Level: Error, Command: full, Message: msg}}
	}
	return nil
}

// verifyScriptLoose is like verifyScript but tolerates dependency binaries
// (pnpm vitest, yarn jest) — they resolve via node_modules/.bin.
func (e *Engine) verifyScriptLoose(name string, file string, line int, full, runner string) []Finding {
	scripts, loc := e.loadScripts()
	if loc == "" {
		return nil
	}
	if _, ok := scripts[name]; ok {
		return nil
	}
	if deps := e.loadDeps(); deps[name] {
		return nil
	}
	nearest := suggestScript(scripts, name)
	msg := runner + " " + quote(name) + " is neither a script in " + loc + " nor a dependency"
	if nearest != "" {
		msg += " (closest script: " + nearest + ")"
	}
	return []Finding{{File: file, Line: line, Level: Warning, Command: full, Message: msg}}
}

func suggestScript(scripts map[string]string, name string) string {
	// exact prefix match or shared prefix of >=3 chars wins
	best := ""
	for k := range scripts {
		if strings.HasPrefix(name, k) || strings.HasPrefix(k, name) {
			if len(k) > len(best) {
				best = k
			}
		}
	}
	return best
}

// candidatePaths returns the paths to try for a root-level config file,
// honoring the active `cd` target of the enclosing fenced block.
func (e *Engine) candidatePaths(name string) []string {
	if e.subDir != "" {
		return []string{filepath.Join(e.subDir, name), name}
	}
	return []string{name}
}

// pkgInfo is the parsed package.json for one working directory.
type pkgInfo struct {
	scripts map[string]string
	deps    map[string]bool
	loc     string
}

func (e *Engine) loadScripts() (map[string]string, string) {
	if pi, ok := e.pkgCache[e.subDir]; ok {
		return pi.scripts, pi.loc
	}
	pi := e.parsePkg()
	if e.pkgCache == nil {
		e.pkgCache = map[string]*pkgInfo{}
	}
	e.pkgCache[e.subDir] = pi
	return pi.scripts, pi.loc
}

func (e *Engine) parsePkg() *pkgInfo {
	pi := &pkgInfo{scripts: map[string]string{}}
	// The package.json matching the fenced block's `cd` target wins first,
	// then the repo root, then the shallowest nested one.
	var candidates []string
	if e.subDir != "" {
		candidates = append(candidates, filepath.Join(e.subDir, "package.json"))
	}
	candidates = append(candidates, "package.json")
	if _, err := os.Stat(filepath.Join(e.Root, "package.json")); err != nil {
		nested := analyze.FindFiles(e.Root, "package.json")
		sort.Slice(nested, func(i, j int) bool {
			return strings.Count(nested[i], "/") < strings.Count(nested[j], "/")
		})
		for _, p := range nested {
			if strings.HasPrefix(p, "node_modules") {
				continue
			}
			candidates = append(candidates, p)
			break
		}
	}
	for _, c := range candidates {
		b, err := os.ReadFile(filepath.Join(e.Root, filepath.FromSlash(c)))
		if err != nil {
			continue
		}
		var pj struct {
			Scripts      map[string]string `json:"scripts"`
			Dependencies map[string]string `json:"dependencies"`
			DevDeps      map[string]string `json:"devDependencies"`
			PeerDeps     map[string]string `json:"peerDependencies"`
			OptionalDeps map[string]string `json:"optionalDependencies"`
		}
		if json.Unmarshal(b, &pj) == nil && len(pj.Scripts) > 0 {
			pi.scripts = pj.Scripts
			pi.loc = c
			deps := map[string]bool{}
			for _, m := range []map[string]string{pj.Dependencies, pj.DevDeps, pj.PeerDeps, pj.OptionalDeps} {
				for name := range m {
					deps[name] = true
					// @scope/name binaries: pnpm exec uses the package name,
					// but many CLIs install under a shorter binary name.
					if i := strings.LastIndex(name, "/"); i >= 0 {
						deps[name[i+1:]] = true
					}
				}
			}
			pi.deps = deps
			break
		}
	}
	return pi
}

func (e *Engine) loadDeps() map[string]bool {
	pi, ok := e.pkgCache[e.subDir]
	if !ok {
		pi = e.parsePkg()
		if e.pkgCache == nil {
			e.pkgCache = map[string]*pkgInfo{}
		}
		e.pkgCache[e.subDir] = pi
	}
	if pi.deps == nil {
		return map[string]bool{}
	}
	return pi.deps
}

func (e *Engine) checkMake(fields []string, file string, line int, full string) []Finding {
	targets := e.loadMake()
	if len(targets) == 0 {
		if e.Facts != nil && (e.Facts.Has("Go") || e.Facts.Has("Rust") || e.Facts.Has("C/C++")) {
			return []Finding{{File: file, Line: line, Level: Warning, Command: full,
				Message: "no Makefile found in repository root"}}
		}
		return nil
	}
	var out []Finding
	for _, a := range fields[1:] {
		if strings.HasPrefix(a, "-") || a == "=" || strings.Contains(a, "=") {
			continue
		}
		if _, ok := targets[a]; !ok {
			out = append(out, Finding{File: file, Line: line, Level: Error, Command: full,
				Message: "make target " + quote(a) + " is not defined in any Makefile"})
		}
	}
	return out
}

func (e *Engine) loadMake() map[string]string {
	if targets, ok := e.makeCache[e.subDir]; ok {
		return targets
	}
	targets := map[string]string{}
	for _, name := range e.candidatePaths("Makefile") {
		if b, err := os.ReadFile(filepath.Join(e.Root, filepath.FromSlash(name))); err == nil {
			for _, t := range extractTargets(string(b)) {
				targets[t] = name
			}
			break
		}
	}
	for _, name := range []string{"makefile", "GNUmakefile"} {
		if len(targets) > 0 {
			break
		}
		for _, p := range e.candidatePaths(name) {
			if b, err := os.ReadFile(filepath.Join(e.Root, filepath.FromSlash(p))); err == nil {
				for _, t := range extractTargets(string(b)) {
					targets[t] = p
				}
				break
			}
		}
	}
	e.makeCache[e.subDir] = targets
	return targets
}

func (e *Engine) checkJust(fields []string, file string, line int, full string) []Finding {
	recipes := e.loadJust()
	if len(recipes) == 0 {
		return nil
	}
	var out []Finding
	skipNext := false
	for _, a := range fields[1:] {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(a, "-") {
			if justValueFlags[a] && !strings.Contains(a, "=") {
				skipNext = true
			}
			continue
		}
		if strings.Contains(a, "=") {
			continue
		}
		if _, ok := recipes[a]; !ok {
			out = append(out, Finding{File: file, Line: line, Level: Error, Command: full,
				Message: "just recipe " + quote(a) + " is not defined in justfile"})
		}
	}
	return out
}

var justValueFlags = map[string]bool{
	"-d": true, "--working-directory": true, "-f": true, "--justfile": true,
	"--shell": true, "--shell-arg": true, "--chooser": true,
	"--color": true, "--dump-format": true,
}

func (e *Engine) loadJust() map[string]string {
	if recipes, ok := e.justCache[e.subDir]; ok {
		return recipes
	}
	recipes := map[string]string{}
	for _, name := range []string{"justfile", "Justfile", ".justfile"} {
		if len(recipes) > 0 {
			break
		}
		for _, p := range e.candidatePaths(name) {
			if b, err := os.ReadFile(filepath.Join(e.Root, filepath.FromSlash(p))); err == nil {
				for _, t := range extractRecipes(string(b)) {
					recipes[t] = p
				}
				break
			}
		}
	}
	e.justCache[e.subDir] = recipes
	return recipes
}

func (e *Engine) checkGo(fields []string, file string, line int, full string) []Finding {
	if len(fields) < 2 {
		return nil
	}
	args := fields[1:]
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		flag := args[0]
		args = args[1:]
		if (flag == "-C" || flag == "-modfile" || flag == "-overlay") && len(args) > 0 {
			args = args[1:]
		}
	}
	if len(args) == 0 {
		return nil
	}
	sub := args[0]
	valid := map[string]bool{
		"build": true, "test": true, "run": true, "vet": true, "fmt": true,
		"mod": true, "work": true, "install": true, "generate": true,
		"tool": true, "version": true, "env": true, "clean": true, "doc": true,
		"list": true, "get": true, "tidy": true,
	}
	var out []Finding
	if !valid[sub] {
		return nil // could be a tool binary (goimports etc.) — stay quiet
	}
	// Validate package path arguments for test/build/vet/list.
	if sub == "test" || sub == "build" || sub == "vet" || sub == "list" {
		skipNext := false
		for _, a := range args[1:] {
			if skipNext {
				skipNext = false
				continue
			}
			if strings.HasPrefix(a, "-") {
				// Flags that consume the next token: -o out, -C dir, ...
				if goValueFlags[a] && !strings.Contains(a, "=") {
					skipNext = true
				}
				continue
			}
			if a == "./..." || a == "..." || a == "." {
				continue
			}
			p := strings.Trim(a, `"'`)
			if !e.dirExists(p) && !e.relExists(p) {
				out = append(out, Finding{File: file, Line: line, Level: Error, Command: full,
					Message: "package path " + quote(p) + " does not exist"})
			}
		}
	}
	if sub == "run" || sub == "test" {
		for _, a := range args[1:] {
			if strings.HasSuffix(a, ".go") && !strings.HasPrefix(a, "-") {
				if !e.relExists(a) {
					out = append(out, Finding{File: file, Line: line, Level: Error, Command: full,
						Message: "file " + quote(a) + " does not exist"})
				}
			}
		}
	}
	return out
}

var goValueFlags = map[string]bool{
	"-C": true, "-asmflags": true, "-buildmode": true, "-compiler": true,
	"-covermode": true, "-coverpkg": true, "-coverprofile": true, "-cpu": true,
	"-exec": true, "-gcflags": true, "-ldflags": true, "-mod": true,
	"-modfile": true, "-o": true, "-overlay": true, "-p": true, "-pkgdir": true,
	"-tags": true, "-toolexec": true, "-vet": true,
	// go test flags whose value would otherwise look like a package path.
	"-benchtime": true, "-blockprofile": true, "-blockprofilerate": true,
	"-count": true, "-fuzz": true,
	"-fuzzminimizetime": true, "-fuzztime": true, "-list": true,
	"-memprofile": true, "-memprofilerate": true, "-mutexprofile": true,
	"-mutexprofilefraction": true, "-outputdir": true, "-parallel": true,
	"-run": true, "-shuffle": true, "-timeout": true, "-trace": true,
}

func (e *Engine) checkCargo(fields []string, file string, line int, full string) []Finding {
	if len(fields) < 2 {
		return nil
	}
	sub := fields[1]
	valid := map[string]bool{
		"build": true, "test": true, "check": true, "run": true, "bench": true,
		"doc": true, "fmt": true, "clippy": true, "add": true, "install": true,
		"publish": true, "clean": true, "tree": true, "nextest": true,
	}
	if !valid[sub] {
		return nil
	}
	// cargo test <path> / cargo run --bin <name> — only validate obvious path args.
	var out []Finding
	for _, a := range fields[2:] {
		if strings.HasPrefix(a, "-") || a == "--" {
			continue
		}
		if strings.HasSuffix(a, ".rs") && !e.relExists(a) {
			out = append(out, Finding{File: file, Line: line, Level: Error, Command: full,
				Message: "file " + quote(a) + " does not exist"})
		}
	}
	return out
}

func (e *Engine) checkPip(fields []string, file string, line int, full string) []Finding {
	var out []Finding
	for i, a := range fields {
		if a == "-r" || a == "--requirement" {
			if i+1 < len(fields) {
				req := strings.Trim(fields[i+1], `"'`)
				if !e.relExists(req) {
					out = append(out, Finding{File: file, Line: line, Level: Error, Command: full,
						Message: "requirements file " + quote(req) + " does not exist"})
				}
			}
		}
	}
	return out
}

func (e *Engine) checkPython(fields []string, file string, line int, full string) []Finding {
	// python -m module / python script.py
	var out []Finding
	for _, a := range fields[1:] {
		if strings.HasPrefix(a, "-") {
			continue
		}
		if strings.HasSuffix(a, ".py") {
			if !e.relExists(a) {
				out = append(out, Finding{File: file, Line: line, Level: Error, Command: full,
					Message: "file " + quote(a) + " does not exist"})
			}
		}
	}
	return out
}

func (e *Engine) checkPytest(fields []string, file string, line int, full string) []Finding {
	var out []Finding
	skipNext := false
	for _, a := range fields[1:] {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(a, "-") {
			if pytestValueFlags[a] && !strings.Contains(a, "=") {
				skipNext = true
			}
			continue
		}
		if strings.Contains(a, "::") {
			a = strings.SplitN(a, "::", 2)[0]
		}
		a = strings.Trim(a, `"'`)
		if !e.relExists(a) && !e.dirExists(a) {
			out = append(out, Finding{File: file, Line: line, Level: Error, Command: full,
				Message: "pytest target " + quote(a) + " does not exist"})
		}
	}
	return out
}

var pytestValueFlags = map[string]bool{
	"-k": true, "-m": true, "--basetemp": true, "--capture": true,
	"--color": true, "--confcutdir": true, "--deselect": true,
	"--durations": true, "--durations-min": true, "--ignore": true,
	"--ignore-glob": true, "--import-mode": true, "--junitxml": true,
	"--maxfail": true, "--override-ini": true, "--rootdir": true,
	"--tb": true, "--verbosity": true,
}

func (e *Engine) checkDocker(fields []string, file string, line int, full string) []Finding {
	var out []Finding
	for i, a := range fields {
		if a == "-f" || a == "--file" {
			if i+1 < len(fields) {
				f := strings.Trim(fields[i+1], `"'`)
				if !e.relExists(f) {
					out = append(out, Finding{File: file, Line: line, Level: Error, Command: full,
						Message: "compose file " + quote(f) + " does not exist"})
				}
			}
		}
	}
	return out
}

// checkScriptArgFiles validates bare file-looking args of tools we don't
// model deeply (eslint src/, tsc --project tsconfig.json, vitest tests/...).
func (e *Engine) checkScriptArgFiles(args []string, file string, line int, full string) []Finding {
	var out []Finding
	for _, a := range args {
		if strings.HasPrefix(a, "-") || strings.Contains(a, "=") {
			continue
		}
		if strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") {
			continue
		}
		a = strings.Trim(a, `"'`)
		if !looksLikeFilePath(a) {
			continue
		}
		if e.relExists(a) || e.dirExists(a) || hasGlob(a) && e.globHasMatches(a) {
			continue
		}
		out = append(out, Finding{File: file, Line: line, Level: Warning, Command: full,
			Message: "path " + quote(a) + " does not exist"})
	}
	return out
}

func hasGlob(p string) bool {
	return strings.ContainsAny(p, "*?[")
}

func (e *Engine) globHasMatches(p string) bool {
	for _, rel := range e.relativeCandidates(p) {
		matches, err := filepath.Glob(filepath.Join(e.Root, filepath.FromSlash(rel)))
		if err == nil && len(matches) > 0 {
			return true
		}
	}
	return false
}

// buildDirs are directories that only exist after install/build; docs may
// reference them legitimately even when absent from a fresh checkout.
var buildDirs = map[string]bool{
	"node_modules": true, "vendor": true, "dist": true, "build": true,
	"target": true, "coverage": true, ".venv": true, "venv": true,
	"__pycache__": true, "site-packages": true,
}

// conceptualFileNames are agent-instruction files that docs mention
// conceptually ("update the CLAUDE.md") far more often than they reference
// as an existing path. A missing one is not a finding.
var conceptualFileNames = map[string]bool{
	"AGENTS.md": true, "CLAUDE.md": true, "GEMINI.md": true,
	"README.md": true, ".cursorrules": true, "copilot-instructions.md": true,
}

var envFileRe = regexp.MustCompile(`^\.env(\.\w+)?$`)

func (e *Engine) checkPathRef(p mdutil.PathRef) []Finding {
	pth := p.Path
	if strings.HasPrefix(pth, "@") {
		return nil
	}
	// A leading "/" in prose usually means repo-root-relative, not
	// filesystem-absolute ("/test/e2e/mock/"). Treat it as relative.
	pth = strings.TrimPrefix(pth, "/")
	// Skip punctuation noise and protocol-relative URLs ("//cdn...").
	if strings.Trim(pth, "/.\\") == "" || strings.HasPrefix(pth, "//") {
		return nil
	}
	// Conceptual mentions of agent-instruction files ("update the
	// CLAUDE.md") are far more common than literal path references.
	if conceptualFileNames[pth] {
		return nil
	}
	// .env files are typically gitignored but real locally — stay silent.
	if envFileRe.MatchString(pth) {
		return nil
	}
	if hasGlob(pth) {
		if !e.globHasMatches(pth) {
			return []Finding{{File: p.File, Line: p.Line, Level: Warning,
				Message: "pattern " + quote(pth) + " matches nothing in the repo"}}
		}
		return nil
	}
	if strings.Contains(pth, "#") { // e.g. README.md#section
		pth = strings.SplitN(pth, "#", 2)[0]
	}
	// "src/**/..." style truncation: validate the prefix directory.
	base := strings.TrimSuffix(pth, "...")
	base = strings.TrimSuffix(base, "/")
	if pth != base && base != "" && e.dirExists(base) {
		return nil
	}
	// References into build output directories can't be validated from a
	// fresh checkout — stay silent.
	firstSeg := pth
	if i := strings.IndexAny(pth, "/\\"); i >= 0 {
		firstSeg = pth[:i]
	}
	if buildDirs[firstSeg] {
		return nil
	}
	if e.relExists(pth) || e.dirExists(pth) {
		return nil
	}
	return []Finding{{File: p.File, Line: p.Line, Level: Warning,
		Message: "referenced file " + quote(pth) + " does not exist"}}
}

func (e *Engine) relExists(p string) bool {
	if filepath.IsAbs(p) {
		return false
	}
	for _, rel := range e.relativeCandidates(p) {
		if _, err := os.Stat(filepath.Join(e.Root, filepath.FromSlash(rel))); err == nil {
			return true
		}
	}
	return false
}

func (e *Engine) dirExists(p string) bool {
	if filepath.IsAbs(p) {
		return false
	}
	for _, rel := range e.relativeCandidates(p) {
		fi, err := os.Stat(filepath.Join(e.Root, filepath.FromSlash(rel)))
		if err == nil && fi.IsDir() {
			return true
		}
	}
	return false
}

func (e *Engine) repoDirExists(rel string) bool {
	if !e.escapeFree(rel) {
		return false
	}
	fi, err := os.Stat(filepath.Join(e.Root, filepath.FromSlash(rel)))
	return err == nil && fi.IsDir()
}

// relativeCandidates resolves references from the instruction file's
// directory/active cd target first, then from the repository root. The
// fallback keeps validation conservative for docs that explicitly state
// their commands run from the repository root.
func (e *Engine) relativeCandidates(p string) []string {
	if !e.escapeFree(p) {
		return nil
	}
	p = filepath.ToSlash(filepath.Clean(p))
	var out []string
	if e.subDir != "" && e.escapeFree(e.subDir) {
		joined := filepath.ToSlash(filepath.Clean(filepath.Join(e.subDir, p)))
		if e.escapeFree(joined) {
			out = append(out, joined)
		}
	}
	if len(out) == 0 || out[0] != p {
		out = append(out, p)
	}
	return out
}

// withinRoot reports whether a repo-relative path stays inside the repo
// after cleaning. References that escape the root ("../shared/x") are not
// validated and not read — agentsmd inspects the checkout only.
func (e *Engine) withinRoot(p string) bool {
	if e.subDir != "" && !e.escapeFree(e.subDir) {
		return false
	}
	return e.escapeFree(p)
}

func (e *Engine) escapeFree(rel string) bool {
	clean := filepath.ToSlash(filepath.Clean(rel))
	return clean == "." || clean == "" || (!strings.HasPrefix(clean, "../") && clean != "..")
}

func looksLikeFilePath(p string) bool {
	if !strings.Contains(p, ".") && !strings.Contains(p, "/") {
		return false
	}
	return strings.Contains(p, "/") || strings.Contains(filepath.Ext(p), ".")
}

// shellFields is a deliberately small shell tokenizer. It keeps quoted flag
// values together (for example `pytest -k "not slow"`) without attempting
// expansion or execution.
func shellFields(s string) []string {
	var fields []string
	var current strings.Builder
	var quote byte
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			fields = append(fields, current.String())
			current.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			current.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				current.WriteByte(c)
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			flush()
			continue
		}
		current.WriteByte(c)
	}
	if escaped {
		current.WriteByte('\\')
	}
	flush()
	return fields
}

func quote(s string) string { return "`" + s + "`" }

// extractTargets pulls make targets from Makefile content.
func extractTargets(content string) []string {
	var out []string
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if strings.HasPrefix(line, "\t") || strings.Contains(line, "=") && !strings.Contains(line, ":") {
			continue
		}
		if strings.Contains(line, ":") {
			head := strings.SplitN(line, ":", 2)[0]
			for _, t := range strings.Fields(head) {
				if t == "" || strings.HasPrefix(t, ".") || strings.Contains(t, "$") {
					continue
				}
				out = append(out, t)
			}
		}
	}
	return out
}

// extractRecipes pulls justfile recipe names.
func extractRecipes(content string) []string {
	var out []string
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, ":") {
			head := strings.SplitN(line, ":", 2)[0]
			f := strings.Fields(head)
			if len(f) >= 1 && !strings.HasPrefix(f[0], ".") {
				out = append(out, f[0])
			}
		}
	}
	return out
}
