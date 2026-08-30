// Package doctor runs every agentsmd health check and prints a report card.
package doctor

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/youwei792/agentsmd/internal/analyze"
	"github.com/youwei792/agentsmd/internal/lint"
	"github.com/youwei792/agentsmd/internal/skills"
	"github.com/youwei792/agentsmd/internal/syncmd"
	"github.com/youwei792/agentsmd/internal/tokens"
	"github.com/youwei792/agentsmd/internal/ui"
)

// Report aggregates all sub-reports for --json output.
type Report struct {
	Root      string          `json:"root"`
	Facts     *analyze.Facts  `json:"facts"`
	Lint      *lint.Report    `json:"lint"`
	Tokens    *tokens.Report  `json:"tokens"`
	SyncState []syncmd.Action `json:"sync"`
	Skills    []skills.Report `json:"skills,omitempty"`
	Score     int             `json:"score"`
	Grade     string          `json:"grade"`
}

// Run executes the doctor command.
func Run(root string, jsonOut bool) int {
	facts, err := analyze.Analyze(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentsmd: %v\n", err)
		return 1
	}
	lr := lint.Run(root, facts)
	tr := tokens.Measure(root)
	sr := syncmd.Run(root, syncmd.Mode("import"), nil, true, "")
	skillReps := skills.Run(root)
	skillsBad := 0
	for _, sk := range skillReps {
		if !sk.Valid {
			skillsBad++
		}
	}

	// Score combines lint (70%) with sync health (30%).
	score := lr.Score
	syncOK := 0
	syncTotal := 0
	for _, a := range sr.Actions {
		syncTotal++
		if a.Status == "up-to-date" {
			syncOK++
		}
	}
	if syncTotal > 0 {
		syncScore := syncOK * 100 / syncTotal
		score = lr.Score*7/10 + syncScore*3/10
	}
	score -= skillsBad * 15
	if score < 0 {
		score = 0
	}

	rep := &Report{Root: root, Facts: facts, Lint: lr, Tokens: tr, SyncState: sr.Actions, Skills: skillReps, Score: score, Grade: lint.Grade(score)}
	if jsonOut {
		return printJSON(rep)
	}

	if len(tr.Files) == 0 {
		fmt.Println(ui.BYellow("⚠ no agent instruction files found"))
		fmt.Println("  " + ui.Dim("run `agentsmd init` to generate a grounded AGENTS.md"))
		return 1
	}

	// Header report card.
	gradeColored := ui.BGreen(rep.Grade)
	if score < 80 {
		gradeColored = ui.BYellow(rep.Grade)
	}
	if score < 60 {
		gradeColored = ui.BRed(rep.Grade)
	}
	fmt.Printf("\n  %s   %s\n\n", ui.Bold("agentsmd health check"), gradeColored+ui.Bold(fmt.Sprintf("  %d/100", score)))

	// Context footprint.
	fmt.Println(ui.Bold("  Context footprint"))
	for _, f := range tr.Files {
		fmt.Printf("    %-38s %s\n", f.Path, ui.Dim(fmt.Sprintf("~%d tokens", f.Tokens)))
	}
	fmt.Printf("    %-38s %s\n", ui.Bold("total"), ui.Bold(fmt.Sprintf("~%d tokens", tr.Total)))
	for _, b := range tokens.ContextBudgets() {
		pct := float64(tr.Total) / float64(b.Size) * 100
		fmt.Printf("    %s\n", ui.Dim(fmt.Sprintf("%.2f%% of a %s context window", pct, b.Name)))
	}

	// Tool bridges.
	if len(sr.Actions) > 0 {
		fmt.Println()
		fmt.Println(ui.Bold("  Tool bridges (AGENTS.md as source of truth)"))
		for _, a := range sr.Actions {
			pretty := strings.NewReplacer(
				"would-create", "not bridged (run `agentsmd sync`)",
				"would-update", "stale bridge (run `agentsmd sync`)",
			).Replace(a.Status)
			switch a.Status {
			case "up-to-date":
				fmt.Printf("    %s %-10s → %s %s\n", ui.Green("✓"), a.Tool, a.Path, ui.Dim("in sync"))
			default:
				fmt.Printf("    %s %-10s → %s %s\n", ui.Yellow("⚠"), a.Tool, a.Path, ui.Dim(pretty))
			}
		}
	}

	// Agent Skills.
	if len(skillReps) > 0 {
		fmt.Println()
		fmt.Println(ui.Bold("  Agent Skills"))
		for _, sk := range skillReps {
			icon := ui.Green("✓")
			if !sk.Valid {
				icon = ui.Red("✗")
			}
			fmt.Printf("    %s %-38s %s\n", icon, sk.Path, ui.Dim(fmt.Sprintf("~%d tokens", sk.Tokens)))
			for _, f := range sk.Findings {
				sev := ui.Yellow("⚠")
				if f.Severity == skills.Err {
					sev = ui.Red("✗")
				}
				fmt.Printf("        %s %s\n", sev, f.Message)
			}
		}
	}

	// Findings, grouped by severity.
	if len(lr.Findings) > 0 {
		fmt.Println()
		fmt.Println(ui.Bold("  Findings"))
		ordered := append([]lint.Finding(nil), lr.Findings...)
		sort.Slice(ordered, func(i, j int) bool { return sevRank(ordered[i].Severity) < sevRank(ordered[j].Severity) })
		for _, f := range ordered {
			icon := ui.Yellow("⚠")
			if f.Severity == lint.Err {
				icon = ui.Red("✗")
			} else if f.Severity == lint.Info {
				icon = ui.Cyan("ℹ")
			}
			fmt.Printf("    %s %-15s %s\n", icon, ui.Dim(f.Rule), wrapText(f.Message, 7))
		}
	}

	// Footer: what to do next.
	fmt.Println()
	switch {
	case score >= 90:
		fmt.Println("  " + ui.BGreen("✓ Agent instructions look healthy."))
	default:
		fmt.Println("  " + ui.Dim("next steps: fill TODOs, fix broken references, then run `agentsmd sync`"))
	}
	fmt.Println()
	return exitFor(score)
}

func exitFor(score int) int {
	if score < 60 {
		return 1
	}
	return 0
}

func sevRank(s lint.Severity) int {
	switch s {
	case lint.Err:
		return 0
	case lint.Warn:
		return 1
	default:
		return 2
	}
}

// wrapText indents continuation lines so the report stays tidy.
func wrapText(s string, indent int) string {
	if len(s) < 110 {
		return s
	}
	pad := strings.Repeat(" ", indent)
	words := strings.Fields(s)
	var out []string
	line := ""
	for _, w := range words {
		if len(line)+len(w) > 104 {
			out = append(out, line)
			line = w
			continue
		}
		if line != "" {
			line += " "
		}
		line += w
	}
	out = append(out, line)
	return strings.Join(out, "\n"+pad)
}

func printJSON(r *Report) int {
	if err := ui.PrintJSON(r); err != nil {
		fmt.Fprintf(os.Stderr, "agentsmd: %v\n", err)
		return 1
	}
	return 0
}
