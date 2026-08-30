// Package checkcmd implements `agentsmd check`: verify that every command
// and file reference in agent instruction files actually exists.
package checkcmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/youwei792/agentsmd/internal/analyze"
	"github.com/youwei792/agentsmd/internal/mdutil"
	"github.com/youwei792/agentsmd/internal/ui"
	"github.com/youwei792/agentsmd/internal/validate"
)

// checkedFiles are the files `check` validates.
var checkedFiles = []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md", ".github/copilot-instructions.md"}

// Run executes the check command. Returns process exit code.
func Run(root string, strict, jsonOut bool) int {
	facts, err := analyze.Analyze(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentsmd: %v\n", err)
		return 1
	}
	eng := validate.NewEngine(root, facts)

	type docResult struct {
		Path     string             `json:"path"`
		Checked  int                `json:"checked"`
		Findings []validate.Finding `json:"findings"`
	}
	var results []docResult
	totalFindings := 0
	totalChecked := 0
	hadDoc := false

	for _, rel := range checkedFiles {
		full := filepath.Join(root, rel)
		b, err := os.ReadFile(full)
		if err != nil || len(b) == 0 {
			continue
		}
		hadDoc = true
		findings := eng.CheckDocument(rel, string(b))
		cmds, paths := countChecked(rel, string(b))
		results = append(results, docResult{Path: rel, Checked: cmds + paths, Findings: findings})
		totalFindings += len(findings)
		totalChecked += cmds + paths
	}

	if !hadDoc {
		if jsonOut {
			ui.PrintJSON(map[string]any{"error": "no AGENTS.md or CLAUDE.md found"})
		} else {
			fmt.Fprintln(os.Stderr, "agentsmd: no AGENTS.md found — run `agentsmd init` first")
		}
		return 1
	}

	if jsonOut {
		type out struct {
			Root      string      `json:"root"`
			Checked   int         `json:"checked_refs"`
			Errors    int         `json:"errors"`
			Warnings  int         `json:"warnings"`
			Documents []docResult `json:"documents"`
		}
		errs, warns := 0, 0
		for _, r := range results {
			for _, f := range r.Findings {
				if f.Level == validate.Error {
					errs++
				} else {
					warns++
				}
			}
		}
		ui.PrintJSON(out{Root: root, Checked: totalChecked, Errors: errs, Warnings: warns, Documents: results})
		if errs > 0 || (strict && warns > 0) {
			return 1
		}
		return 0
	}

	fmt.Println(ui.Bold("Checking agent instruction files:") + ui.Dim(fmt.Sprintf(" %d reference(s)", totalChecked)))
	for _, r := range results {
		if len(r.Findings) == 0 {
			fmt.Printf("  %s %s\n", ui.Green("✓"), r.Path)
			continue
		}
		fmt.Printf("  %s %s\n", ui.Red("✗"), r.Path)
		for _, f := range r.Findings {
			icon := ui.Yellow("⚠")
			if f.Level == validate.Error {
				icon = ui.Red("✗")
			}
			fmt.Printf("      %s AGENTS.md:%d  %s\n", icon, f.Line, f.Message)
			if f.Command != "" {
				fmt.Printf("         %s %s\n", ui.Dim("in:"), ui.Dim(f.Command))
			}
		}
	}
	errs, warns := 0, 0
	for _, r := range results {
		for _, f := range r.Findings {
			if f.Level == validate.Error {
				errs++
			} else {
				warns++
			}
		}
	}
	fmt.Println()
	if totalFindings == 0 {
		fmt.Println(ui.BGreen("✓ all references are real") + ui.Dim(fmt.Sprintf(" (%d checked)", totalChecked)))
		return 0
	}
	summary := fmt.Sprintf("%d error(s), %d warning(s)", errs, warns)
	if errs > 0 {
		fmt.Println(ui.BRed("✗ " + summary))
	} else {
		fmt.Println(ui.BYellow("⚠ " + summary))
		if !strict {
			return 0
		}
	}
	if strict && warns > 0 {
		return 1
	}
	return 1
}

func countChecked(file, content string) (cmds, paths int) {
	_, cs, ps := mdutil.Parse(file, content)
	return len(cs), len(ps)
}
