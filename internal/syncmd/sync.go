// Package sync keeps tool-specific instruction files aligned with AGENTS.md.
// Claude Code does not read AGENTS.md natively, so the recommended bridge is
// a one-line `@AGENTS.md` import inside CLAUDE.md (per Anthropic docs) — no
// fragile symlinks, no duplicated content.
package syncmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Mode selects how tool files are bridged to AGENTS.md.
type Mode string

const (
	ModeImport  Mode = "import"  // one-line @AGENTS.md import (default)
	ModeCopy    Mode = "copy"    // full copy of content
	ModeSymlink Mode = "symlink" // filesystem symlink
)

// Target is one bridgeable file.
type Target struct {
	Tool string // claude | gemini
	Path string
}

// Targets lists the bridges agentsmd manages.
func Targets() []Target {
	return []Target{
		{Tool: "claude", Path: "CLAUDE.md"},
		{Tool: "gemini", Path: "GEMINI.md"},
	}
}

// Action describes what sync did (or would do) for one target.
type Action struct {
	Tool   string `json:"tool"`
	Path   string `json:"path"`
	Status string `json:"status"` // created | updated | up-to-date | would-create | would-update
	Detail string `json:"detail,omitempty"`
	Mode   string `json:"mode"`
}

// Result is the aggregate outcome.
type Result struct {
	Actions    []Action `json:"actions"`
	InSync     bool     `json:"in_sync"`
	ExitErrors []string `json:"exit_errors,omitempty"`
}

const header = "<!-- managed by agentsmd: this file bridges to AGENTS.md. Edit AGENTS.md instead. -->"

// Run performs (or, with checkOnly, simulates) the sync.
func Run(root string, mode Mode, tools []string, checkOnly bool, copyBody string) *Result {
	res := &Result{InSync: true}
	if mode == "" {
		mode = ModeImport
	}

	for _, t := range Targets() {
		if len(tools) > 0 && !contains(tools, t.Tool) {
			continue
		}
		acts := syncTarget(root, t, mode, checkOnly, copyBody)
		res.Actions = append(res.Actions, acts...)
	}

	for _, a := range res.Actions {
		if strings.HasPrefix(a.Status, "would-") {
			res.InSync = false
		}
	}
	return res
}

func syncTarget(root string, t Target, mode Mode, checkOnly bool, copyBody string) []Action {
	full := filepath.Join(root, t.Path)
	existing := readIfExists(full)

	switch mode {
	case ModeImport:
		return syncImport(t, full, existing, checkOnly)
	case ModeCopy:
		return syncCopy(t, full, existing, checkOnly, copyBody)
	case ModeSymlink:
		return syncSymlink(t, full, existing, checkOnly, root)
	}
	return nil
}

func syncImport(t Target, full, existing string, checkOnly bool) []Action {
	if strings.Contains(existing, "@AGENTS.md") {
		return []Action{{Tool: t.Tool, Path: t.Path, Status: "up-to-date", Mode: "import"}}
	}
	status := "created"
	detail := "@" + "AGENTS.md"
	if existing != "" {
		status = "updated"
		detail = "added @AGENTS.md import to existing content"
	}
	if checkOnly {
		if status == "created" {
			status = "would-create"
		} else {
			status = "would-update"
		}
	} else {
		if err := writeImport(full, existing); err != nil {
			return []Action{{Tool: t.Tool, Path: t.Path, Status: "error", Detail: err.Error(), Mode: "import"}}
		}
	}
	return []Action{{Tool: t.Tool, Path: t.Path, Status: status, Detail: detail, Mode: "import"}}
}

// writeImport creates or updates path with an @AGENTS.md import line.
// For existing files, the import is added as the first non-comment line so
// the agent loads AGENTS.md before any file-specific notes.
func writeImport(full, existing string) error {
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if existing == "" {
		content := header + "\n\n@AGENTS.md\n"
		return os.WriteFile(full, []byte(content), 0o644)
	}
	// Insert after leading comment lines, else at top.
	lines := strings.Split(existing, "\n")
	i := 0
	for i < len(lines) && (strings.HasPrefix(strings.TrimSpace(lines[i]), "<!--") || strings.TrimSpace(lines[i]) == "") {
		if strings.Contains(lines[i], "-->") && strings.HasPrefix(strings.TrimSpace(lines[i]), "<!--") {
			i++
			break
		}
		i++
		if i < len(lines) && strings.TrimSpace(lines[i]) != "" && !strings.HasPrefix(strings.TrimSpace(lines[i]), "<!--") {
			break
		}
	}
	var out []string
	out = append(out, lines[:i]...)
	out = append(out, "", header, "", "@AGENTS.md", "")
	out = append(out, lines[i:]...)
	return os.WriteFile(full, []byte(strings.Join(out, "\n")), 0o644)
}

func syncCopy(t Target, full, existing string, checkOnly bool, body string) []Action {
	if body == "" {
		return []Action{{Tool: t.Tool, Path: t.Path, Status: "error", Detail: "no AGENTS.md content provided", Mode: "copy"}}
	}
	managed := strings.HasPrefix(existing, "<!-- managed by agentsmd")
	if strings.TrimSpace(existing) == strings.TrimSpace(body) {
		return []Action{{Tool: t.Tool, Path: t.Path, Status: "up-to-date", Mode: "copy"}}
	}
	status := "created"
	if existing != "" && managed {
		status = "updated"
	}
	if existing != "" && !managed {
		return []Action{{Tool: t.Tool, Path: t.Path, Status: "error", Mode: "copy",
			Detail: fmt.Sprintf("%s exists and is not agentsmd-managed; refusing to overwrite (use import mode)", t.Path)}}
	}
	if checkOnly {
		if status == "created" {
			status = "would-create"
		} else {
			status = "would-update"
		}
	} else {
		content := header + "\n\n" + body
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return []Action{{Tool: t.Tool, Path: t.Path, Status: "error", Detail: err.Error(), Mode: "copy"}}
		}
	}
	return []Action{{Tool: t.Tool, Path: t.Path, Status: status, Mode: "copy"}}
}

func syncSymlink(t Target, full, existing string, checkOnly bool, root string) []Action {
	agentsPath := filepath.Join(root, "AGENTS.md")
	if fi, err := os.Lstat(full); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			dest, err := os.Readlink(full)
			if err == nil && dest == "AGENTS.md" {
				return []Action{{Tool: t.Tool, Path: t.Path, Status: "up-to-date", Mode: "symlink"}}
			}
		} else {
			return []Action{{Tool: t.Tool, Path: t.Path, Status: "error", Mode: "symlink",
				Detail: fmt.Sprintf("%s exists as a regular file; refusing to replace it", t.Path)}}
		}
	}
	if checkOnly {
		return []Action{{Tool: t.Tool, Path: t.Path, Status: "would-create", Mode: "symlink"}}
	}
	if err := os.Symlink("AGENTS.md", full); err != nil {
		return []Action{{Tool: t.Tool, Path: t.Path, Status: "error", Detail: err.Error(), Mode: "symlink"}}
	}
	_ = agentsPath
	return []Action{{Tool: t.Tool, Path: t.Path, Status: "created", Mode: "symlink"}}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func readIfExists(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
