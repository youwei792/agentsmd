// Package analyze detects the toolchain facts of a repository: package
// managers, frameworks, scripts, monorepo layout, linters, CI commands and
// existing agent-instruction files. Everything agentsmd builds on top of it
// (generation, checking, linting) is grounded in these facts.
package analyze

// Command is a command discovered in the repository (package.json script,
// Makefile target, justfile recipe, ...).
type Command struct {
	Name     string `json:"name"`                // e.g. "test", "build"
	Cmdline  string `json:"cmdline"`             // e.g. "npm run test"
	Source   string `json:"source"`              // file that defines it
	Purpose  string `json:"purpose,omitempty"`   // build | test | lint | format | dev | run | other
	IsScript bool   `json:"is_script,omitempty"` // true for npm/poetry-style named scripts
}

// ToolchainFile records a detected config file with what it told us.
type ToolchainFile struct {
	Path   string `json:"path"`
	Detail string `json:"detail,omitempty"`
}

// MonorepoInfo describes a detected multi-package layout.
type MonorepoInfo struct {
	Kind     string   `json:"kind"` // npm-workspaces | pnpm-workspace | bun-workspaces | go-workspace | cargo-workspace | turborepo | nx
	Manifest string   `json:"manifest"`
	Packages []string `json:"packages"`
}

// Framework is a detected framework or major dependency.
type Framework struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Source  string `json:"source,omitempty"`
}

// AgentFile is an existing agent-instruction file in the repo.
type AgentFile struct {
	Path   string `json:"path"`
	Tool   string `json:"tool"` // agents | claude | gemini | cursor | copilot | windsurf | cline | aider
	Bytes  int    `json:"bytes"`
	HasRef bool   `json:"has_ref"` // references AGENTS.md (e.g. "@AGENTS.md" import)
}

// Facts is the result of analyzing a repository.
type Facts struct {
	Root           string          `json:"root"`
	Languages      []string        `json:"languages"`
	PackageMgrs    []string        `json:"package_managers"` // primary first
	Frameworks     []Framework     `json:"frameworks"`
	Scripts        []Command       `json:"scripts"`
	Monorepo       *MonorepoInfo   `json:"monorepo,omitempty"`
	Linters        []string        `json:"linters"`
	Formatters     []string        `json:"formatters"`
	TestFrameworks []string        `json:"test_frameworks"`
	CICommands     []string        `json:"ci_commands"`
	Docker         bool            `json:"docker"`
	AgentFiles     []AgentFile     `json:"agent_files"`
	ConfigFiles    []ToolchainFile `json:"config_files"`
	Manifests      []string        `json:"manifests"` // files that make staleness checks meaningful
	Warnings       []string        `json:"warnings"`
}

// Has reports whether the language was detected.
func (f *Facts) Has(lang string) bool {
	for _, l := range f.Languages {
		if l == lang {
			return true
		}
	}
	return false
}

// PackageManager returns the primary package manager, or "".
func (f *Facts) PackageManager() string {
	if len(f.PackageMgrs) > 0 {
		return f.PackageMgrs[0]
	}
	return ""
}

// Script returns the command whose Purpose matches (first match).
func (f *Facts) Script(purpose string) *Command {
	for i := range f.Scripts {
		if f.Scripts[i].Purpose == purpose {
			return &f.Scripts[i]
		}
	}
	return nil
}

// AgentFile returns the detected file for the given tool, if any.
func (f *Facts) GetAgentFile(tool string) *AgentFile {
	for i := range f.AgentFiles {
		if f.AgentFiles[i].Tool == tool {
			return &f.AgentFiles[i]
		}
	}
	return nil
}
