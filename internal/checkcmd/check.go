// Package checkcmd implements `agentsmd check`: verify that every command
// and file reference in agent instruction files actually exists.
package checkcmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/youwei792/agentsmd/internal/analyze"
	"github.com/youwei792/agentsmd/internal/mdutil"
	"github.com/youwei792/agentsmd/internal/safeio"
	"github.com/youwei792/agentsmd/internal/ui"
	"github.com/youwei792/agentsmd/internal/validate"
)

// DocumentResult is the validation result for one instruction document.
type DocumentResult struct {
	Path     string             `json:"path"`
	Checked  int                `json:"checked"`
	Findings []validate.Finding `json:"findings"`
}

// Report is the complete machine-readable result of a check run.
type Report struct {
	Root      string           `json:"root"`
	Checked   int              `json:"checked_refs"`
	Errors    int              `json:"errors"`
	Warnings  int              `json:"warnings"`
	Documents []DocumentResult `json:"documents"`
}

// Failed reports whether this check should fail at the requested strictness.
func (r *Report) Failed(strict bool) bool {
	return r.Errors > 0 || strict && r.Warnings > 0
}

var instructionNames = map[string]bool{
	"AGENTS.md": true,
	"CLAUDE.md": true,
	"GEMINI.md": true,
}

var skippedDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true,
	"node_modules": true, "vendor": true,
	"dist": true, "build": true, "target": true, "coverage": true,
	".venv": true, "venv": true, "__pycache__": true,
	".tox": true, ".mypy_cache": true, ".pytest_cache": true,
	".ruff_cache": true, ".next": true, ".turbo": true,
	".nuxt": true, ".output": true,
}

// Inspect validates all repository instruction documents without printing.
// Nested AGENTS.md/CLAUDE.md/GEMINI.md files are included because they govern
// scoped subtrees just as directly as the root document governs the repo.
func Inspect(root string) (*Report, error) {
	facts, err := analyze.Analyze(root)
	if err != nil {
		return nil, err
	}
	docs, err := discoverDocuments(root)
	if err != nil {
		return nil, err
	}

	rep := &Report{Root: root}
	eng := validate.NewEngine(root, facts)
	for _, rel := range docs {
		b, err := safeio.ReadFileWithin(root, rel)
		result := DocumentResult{Path: rel}
		if err != nil {
			result.Findings = []validate.Finding{{
				File: rel, Line: 1, Level: validate.Error,
				Message: "cannot safely read instruction file: " + err.Error(),
			}}
		} else {
			result.Findings = eng.CheckDocument(rel, string(b))
			cmds, paths := countChecked(rel, string(b))
			result.Checked = cmds + paths
			rep.Checked += result.Checked
		}
		for _, finding := range result.Findings {
			if finding.Level == validate.Error {
				rep.Errors++
			} else {
				rep.Warnings++
			}
		}
		rep.Documents = append(rep.Documents, result)
	}
	return rep, nil
}

func discoverDocuments(root string) ([]string, error) {
	var docs []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.IsDir() {
			if skippedDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !instructionNames[entry.Name()] {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		docs = append(docs, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Copilot's conventional repository-wide instructions use a distinct
	// filename, so add that one explicitly.
	copilot := filepath.Join(root, ".github", "copilot-instructions.md")
	if _, err := os.Lstat(copilot); err == nil {
		docs = append(docs, ".github/copilot-instructions.md")
	}
	sort.Slice(docs, func(i, j int) bool {
		depthI := strings.Count(docs[i], "/")
		depthJ := strings.Count(docs[j], "/")
		if depthI != depthJ {
			return depthI < depthJ
		}
		return docs[i] < docs[j]
	})
	return docs, nil
}

// Run executes the check command. Returns process exit code.
func Run(root string, strict, jsonOut bool) int {
	rep, err := Inspect(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentsmd: %v\n", err)
		return 1
	}
	if len(rep.Documents) == 0 {
		if jsonOut {
			if err := ui.PrintJSON(map[string]any{"error": "no AGENTS.md, CLAUDE.md, or GEMINI.md found"}); err != nil {
				fmt.Fprintf(os.Stderr, "agentsmd: %v\n", err)
			}
		} else {
			fmt.Fprintln(os.Stderr, "agentsmd: no AGENTS.md found — run `agentsmd init` first")
		}
		return 1
	}

	if jsonOut {
		if err := ui.PrintJSON(rep); err != nil {
			fmt.Fprintf(os.Stderr, "agentsmd: %v\n", err)
			return 1
		}
		if rep.Failed(strict) {
			return 1
		}
		return 0
	}

	fmt.Println(ui.Bold("Checking agent instruction files:") + ui.Dim(fmt.Sprintf(" %d reference(s)", rep.Checked)))
	for _, result := range rep.Documents {
		if len(result.Findings) == 0 {
			fmt.Printf("  %s %s\n", ui.Green("✓"), result.Path)
			continue
		}
		fmt.Printf("  %s %s\n", ui.Red("✗"), result.Path)
		for _, finding := range result.Findings {
			icon := ui.Yellow("⚠")
			if finding.Level == validate.Error {
				icon = ui.Red("✗")
			}
			file := finding.File
			if file == "" {
				file = result.Path
			}
			fmt.Printf("      %s %s:%d  %s\n", icon, file, finding.Line, finding.Message)
			if finding.Command != "" {
				fmt.Printf("         %s %s\n", ui.Dim("in:"), ui.Dim(finding.Command))
			}
		}
	}

	fmt.Println()
	if rep.Errors == 0 && rep.Warnings == 0 {
		fmt.Println(ui.BGreen("✓ all references are real") + ui.Dim(fmt.Sprintf(" (%d checked)", rep.Checked)))
		return 0
	}
	summary := fmt.Sprintf("%d error(s), %d warning(s)", rep.Errors, rep.Warnings)
	if rep.Errors > 0 {
		fmt.Println(ui.BRed("✗ " + summary))
	} else {
		fmt.Println(ui.BYellow("⚠ " + summary))
		if !strict {
			return 0
		}
	}
	if rep.Failed(strict) {
		return 1
	}
	return 0
}

func countChecked(file, content string) (cmds, paths int) {
	_, cs, ps := mdutil.Parse(file, content)
	return len(cs), len(ps)
}
