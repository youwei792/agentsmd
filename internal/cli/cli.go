// Package cli wires the subcommands together.
package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/youwei792/agentsmd/internal/analyze"
	"github.com/youwei792/agentsmd/internal/checkcmd"
	"github.com/youwei792/agentsmd/internal/doctor"
	"github.com/youwei792/agentsmd/internal/fleet"
	"github.com/youwei792/agentsmd/internal/generate"
	"github.com/youwei792/agentsmd/internal/lint"
	"github.com/youwei792/agentsmd/internal/skills"
	"github.com/youwei792/agentsmd/internal/syncmd"
	"github.com/youwei792/agentsmd/internal/tokens"
	"github.com/youwei792/agentsmd/internal/ui"
)

// version is overridden at release build time via
// -ldflags "-X github.com/youwei792/agentsmd/internal/cli.version=<tag>".
var version = "dev"

const usage = `agentsmd — keep AI agent instructions as healthy as your code

Usage:
  agentsmd <command> [path] [flags]

Commands:
  init      generate a grounded AGENTS.md from repository facts
  check     verify every command & file AGENTS.md references exists
  lint      audit quality, staleness and consistency (scored)
  tokens    estimate the context cost of agent instruction files
  sync      bridge AGENTS.md to CLAUDE.md / GEMINI.md
  doctor    run check + lint + tokens + sync-check in one report
  skills    validate Agent Skills (SKILL.md) bundles in this repo
  org       fleet report: AGENTS.md health of every repo of an org/user (needs gh)
  analyze   show the toolchain facts agentsmd detects (debug/demo)
  version   print version

Run "agentsmd <command> -h" for per-command flags.`

// Run executes the CLI and returns the process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Println(usage)
		return 0
	}
	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "init":
		return cmdInit(rest)
	case "check":
		return cmdCheck(rest)
	case "lint":
		return cmdLint(rest)
	case "tokens":
		return cmdTokens(rest)
	case "sync":
		return cmdSync(rest)
	case "doctor":
		return cmdDoctor(rest)
	case "analyze":
		return cmdAnalyze(rest)
	case "skills":
		return cmdSkills(rest)
	case "org":
		return cmdOrg(rest)
	case "version", "--version", "-v":
		fmt.Printf("agentsmd %s\n", reportedVersion())
		return 0
	case "help", "--help", "-h":
		fmt.Println(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "agentsmd: unknown command %q\n\n%s\n", cmd, usage)
		return 2
	}
}

func reportedVersion() string {
	if version != "dev" {
		return strings.TrimPrefix(version, "v")
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		moduleVersion := info.Main.Version
		if moduleVersion != "" && moduleVersion != "(devel)" {
			return strings.TrimPrefix(moduleVersion, "v")
		}
	}
	return version
}

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	minimal := fs.Bool("minimal", false, "skeleton only, no detected commands")
	force := fs.Bool("force", false, "overwrite existing AGENTS.md (backs up to AGENTS.md.bak)")
	dryRun := fs.Bool("dry-run", false, "print the generated file instead of writing it")
	fs.Parse(normalizeArgs(args))

	root := resolvePath(fs.Arg(0))
	facts, err := analyze.Analyze(root)
	if err != nil {
		return fatal(err)
	}
	res := generate.Build(facts, generate.Options{Minimal: *minimal})

	if *dryRun {
		fmt.Print(res.Content)
		return 0
	}

	target := filepath.Join(root, "AGENTS.md")
	if _, err := os.Stat(target); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "agentsmd: %s already exists (use --force to replace, a backup is kept)\n", target)
		return 1
	}
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, target+".bak"); err != nil {
			return fatal(err)
		}
		fmt.Println(ui.Dim("existing AGENTS.md backed up to AGENTS.md.bak"))
	}
	if err := os.WriteFile(target, []byte(res.Content), 0o644); err != nil {
		return fatal(err)
	}

	fmt.Println(ui.BGreen("✓") + " wrote " + target)
	fmt.Println()
	fmt.Println(ui.Bold("Detected:"))
	for _, d := range res.Detected {
		fmt.Println("  • " + d)
	}
	fmt.Println()
	fmt.Printf("%s ~%d tokens, %s\n", ui.Bold("Generated:"), res.Tokens, ui.Dim(fmt.Sprintf("%d sections, %d TODO(s) to fill in", len(res.Sections), res.TODOs)))
	if res.TODOs > 0 {
		fmt.Println()
		fmt.Println(ui.Yellow("→") + " Next: fill in the TODOs, then run " + ui.Bold("agentsmd check") + " to verify every command.")
	}
	return 0
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	strict := fs.Bool("strict", false, "treat warnings as failures")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Parse(normalizeArgs(args))

	root := resolvePath(fs.Arg(0))
	return checkcmd.Run(root, *strict, *jsonOut)
}

func cmdLint(args []string) int {
	fs := flag.NewFlagSet("lint", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Parse(normalizeArgs(args))

	root := resolvePath(fs.Arg(0))
	facts, err := analyze.Analyze(root)
	if err != nil {
		return fatal(err)
	}
	rep := lint.Run(root, facts)
	if *jsonOut {
		if code := printJSON(rep); code != 0 {
			return code
		}
		return exitCodeFor(rep.Score, len(rep.Findings))
	}
	for _, f := range rep.Findings {
		icon := ui.Yellow("⚠")
		if f.Severity == lint.Err {
			icon = ui.Red("✗")
		} else if f.Severity == lint.Info {
			icon = ui.Cyan("ℹ")
		}
		fmt.Printf("  %s  %-16s %s\n", icon, ui.Dim(f.Rule), f.Message)
		if f.Hint != "" {
			fmt.Printf("       %s %s\n", ui.Dim("↳"), ui.Dim(f.Hint))
		}
	}
	fmt.Println()
	gradeColored := ui.Green(rep.Grade)
	if rep.Score < 80 {
		gradeColored = ui.Yellow(rep.Grade)
	}
	if rep.Score < 60 {
		gradeColored = ui.Red(rep.Grade)
	}
	fmt.Printf("  Score: %s %s\n", ui.Bold(fmt.Sprintf("%d/100", rep.Score)), gradeColored)
	if rep.Score < 100 {
		fmt.Println("  " + ui.Dim("run with --json for machine-readable output"))
	}
	return exitCodeFor(rep.Score, len(rep.Findings))
}

func cmdTokens(args []string) int {
	fs := flag.NewFlagSet("tokens", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Parse(normalizeArgs(args))

	root := resolvePath(fs.Arg(0))
	rep := tokens.Measure(root)
	if *jsonOut {
		if code := printJSON(rep); code != 0 {
			return code
		}
		if len(rep.Files) == 0 {
			return 1
		}
		return 0
	}
	if len(rep.Files) == 0 {
		fmt.Println(ui.Yellow("⚠") + " no agent instruction files found")
		return 1
	}
	fmt.Println(ui.Bold("Agent context footprint:"))
	for _, f := range rep.Files {
		fmt.Printf("  %-36s %s\n", f.Path, ui.Dim(fmt.Sprintf("~%5d tokens  (%s)", f.Tokens, humanBytes(f.Bytes))))
	}
	fmt.Printf("\n  %-36s %s\n", ui.Bold("total"), ui.Bold(fmt.Sprintf("~%d tokens", rep.Total)))
	for _, b := range tokens.ContextBudgets() {
		pct := float64(rep.Total) / float64(b.Size) * 100
		fmt.Printf("  %s\n", ui.Dim(fmt.Sprintf("%5.2f%% of a %s context window", pct, b.Name)))
	}
	return 0
}

func cmdSync(args []string) int {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	mode := fs.String("mode", "import", "bridge mode: import | copy | symlink")
	checkOnly := fs.Bool("check", false, "only verify bridges are up to date (CI-friendly, exits 1 if stale)")
	tools := fs.String("tools", "", "comma-separated subset: claude,gemini")
	fs.Parse(normalizeArgs(args))

	root := resolvePath(fs.Arg(0))
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		fmt.Fprintln(os.Stderr, "agentsmd: no AGENTS.md found — run `agentsmd init` first")
		return 1
	}
	var toolList []string
	if *tools != "" {
		for _, t := range strings.Split(*tools, ",") {
			toolList = append(toolList, strings.TrimSpace(t))
		}
	}
	copyBody := ""
	if syncmd.Mode(*mode) == syncmd.ModeCopy {
		body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
		if err != nil {
			return fatal(err)
		}
		copyBody = string(body)
	}
	res := syncmd.Run(root, syncmd.Mode(*mode), toolList, *checkOnly, copyBody)
	pretty := strings.NewReplacer("would-create", "would create", "would-update", "would update")
	for _, a := range res.Actions {
		switch {
		case a.Status == "up-to-date":
			fmt.Printf("  %s %-8s %-12s %s\n", ui.Green("✓"), a.Tool, a.Path, ui.Dim(a.Status))
		case a.Status == "created" || a.Status == "updated":
			fmt.Printf("  %s %-8s %-12s %s\n", ui.Green("✓"), a.Tool, a.Path, a.Status)
		case strings.HasPrefix(a.Status, "would-"):
			fmt.Printf("  %s %-8s %-12s %s\n", ui.Yellow("⚠"), a.Tool, a.Path, pretty.Replace(a.Status))
		default:
			fmt.Printf("  %s %-8s %-12s %s\n", ui.Red("✗"), a.Tool, a.Path, a.Status)
			if a.Detail != "" {
				fmt.Printf("       %s\n", ui.Dim(a.Detail))
			}
		}
	}
	if len(res.ExitErrors) > 0 {
		for _, msg := range res.ExitErrors {
			fmt.Fprintln(os.Stderr, "agentsmd: "+msg)
		}
		return 1
	}
	if !res.InSync && *checkOnly {
		fmt.Fprintln(os.Stderr, "\nagentsmd: sync is out of date")
		return 1
	}
	return 0
}

func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Parse(normalizeArgs(args))

	root := resolvePath(fs.Arg(0))
	return doctor.Run(root, *jsonOut)
}

func cmdAnalyze(args []string) int {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Parse(normalizeArgs(args))

	root := resolvePath(fs.Arg(0))
	facts, err := analyze.Analyze(root)
	if err != nil {
		return fatal(err)
	}
	if *jsonOut {
		return printJSON(facts)
	}
	fmt.Println(ui.Bold("Toolchain facts") + ui.Dim("  ("+root+")"))
	fmt.Printf("  languages         %s\n", joinOr(facts.Languages))
	fmt.Printf("  package managers  %s\n", joinOr(facts.PackageMgrs))
	fmt.Printf("  test frameworks   %s\n", joinOr(facts.TestFrameworks))
	fmt.Printf("  linters           %s\n", joinOr(facts.Linters))
	fmt.Printf("  formatters        %s\n", joinOr(facts.Formatters))
	if facts.Monorepo != nil {
		fmt.Printf("  monorepo          %s (%s)\n", facts.Monorepo.Kind, facts.Monorepo.Manifest)
	}
	fmt.Printf("  docker            %t\n", facts.Docker)
	if len(facts.Frameworks) > 0 {
		fmt.Println("  frameworks")
		for _, fw := range facts.Frameworks {
			v := fw.Version
			if v != "" {
				v = " " + v
			}
			fmt.Printf("    • %s%s\n", fw.Name, ui.Dim(v))
		}
	}
	if len(facts.Scripts) > 0 {
		fmt.Println("  scripts")
		for _, s := range facts.Scripts {
			fmt.Printf("    • %-28s %s\n", s.Cmdline, ui.Dim(s.Source))
		}
	}
	if len(facts.AgentFiles) > 0 {
		fmt.Println("  agent files")
		for _, a := range facts.AgentFiles {
			extra := ""
			if a.HasRef {
				extra = ui.Green(" (imports AGENTS.md)")
			}
			fmt.Printf("    • %-28s %s%s\n", a.Path, ui.Dim(humanBytes(a.Bytes)), extra)
		}
	}
	for _, w := range facts.Warnings {
		fmt.Println("  " + ui.Yellow("⚠") + " " + w)
	}
	return 0
}

func cmdSkills(args []string) int {
	fs := flag.NewFlagSet("skills", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Parse(normalizeArgs(args))

	root := resolvePath(fs.Arg(0))
	reports := skills.Run(root)
	bad := 0
	for _, r := range reports {
		if !r.Valid {
			bad++
		}
	}
	if *jsonOut {
		if code := printJSON(reports); code != 0 {
			return code
		}
		if bad > 0 {
			return 1
		}
		return 0
	}
	if len(reports) == 0 {
		fmt.Println(ui.Dim("no SKILL.md bundles found (looked in .claude/skills/ and skills/)"))
		return 0
	}
	fmt.Println(ui.Bold("Agent Skills:"))
	for _, r := range reports {
		icon := ui.Green("✓")
		if !r.Valid {
			icon = ui.Red("✗")
			bad++
		}
		fmt.Printf("  %s %-40s %s\n", icon, r.Path, ui.Dim(fmt.Sprintf("~%d tokens", r.Tokens)))
		for _, f := range r.Findings {
			sev := ui.Yellow("⚠")
			if f.Severity == skills.Err {
				sev = ui.Red("✗")
			}
			fmt.Printf("      %s %s\n", sev, f.Message)
		}
	}
	total := 0
	for _, r := range reports {
		total += r.Tokens
	}
	fmt.Printf("\n  %s skills, ~%d tokens loaded on demand\n", ui.Bold(fmt.Sprintf("%d", len(reports))), total)
	if bad > 0 {
		return 1
	}
	return 0
}

func cmdOrg(args []string) int {
	fs := flag.NewFlagSet("org", flag.ExitOnError)
	limit := fs.Int("limit", 30, "max repositories to scan")
	jsonOut := fs.Bool("json", false, "output JSON")
	fs.Parse(normalizeArgs(args))

	owner := fs.Arg(0)
	if owner == "" {
		fmt.Fprintln(os.Stderr, "usage: agentsmd org <owner> [--limit N]")
		return 2
	}
	rep, err := fleet.Scan("gh", owner, *limit)
	if err != nil {
		return fatal(err)
	}
	if *jsonOut {
		return printJSON(rep)
	}
	fmt.Printf("\n%s%s%s\n",
		ui.Bold("Fleet report: "), ui.Bold(owner),
		ui.Dim(fmt.Sprintf("  %d/%d public repos ship AGENTS.md", rep.ReposHit, rep.ReposSeen)))
	if len(rep.Reports) == 0 {
		fmt.Println("  " + ui.Dim("no root AGENTS.md files found"))
		return 0
	}
	for _, r := range rep.Reports {
		flags := ""
		switch {
		case r.TODOs > 0:
			flags = ui.Yellow(fmt.Sprintf("  %d TODO(s)", r.TODOs))
		case r.CodeRefs == 0:
			flags = ui.Yellow("  no commands documented")
		}
		fmt.Printf("  %-44s ~%5d tokens%s\n", r.Repo, r.Tokens, flags)
	}
	fmt.Printf("\n  fleet context cost: %s\n", ui.Bold(fmt.Sprintf("~%d tokens per full org onboarding", rep.TotalTokens)))
	return 0
}

// ---- helpers ----

// boolFlags and valueFlags are known across all subcommands. Because Go's
// flag package stops parsing at the first positional argument, we hoist
// flags in front of positionals so both `agentsmd tokens --json .` and
// `agentsmd tokens . --json` work.
var boolFlags = map[string]bool{
	"--json": true, "--strict": true, "--minimal": true,
	"--force": true, "--dry-run": true, "--check": true,
	"-h": true, "--help": true,
}

var valueFlags = map[string]bool{"--mode": true, "--tools": true, "--limit": true}

func normalizeArgs(args []string) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if valueFlags[a] && i+1 < len(args) {
			flags = append(flags, a, args[i+1])
			i++
			continue
		}
		if boolFlags[a] || strings.HasPrefix(a, "-") && len(a) > 1 && !isNumericArg(a) {
			flags = append(flags, a)
			continue
		}
		rest = append(rest, a)
	}
	return append(flags, rest...)
}

func isNumericArg(a string) bool {
	if a == "-" {
		return true
	}
	body := strings.TrimPrefix(a, "-")
	if body == "" {
		return false
	}
	for _, c := range body {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func resolvePath(arg string) string {
	if arg == "" {
		return "."
	}
	return arg
}

func fatal(err error) int {
	fmt.Fprintf(os.Stderr, "agentsmd: %v\n", err)
	return 1
}

func printJSON(v any) int {
	if err := ui.PrintJSON(v); err != nil {
		return fatal(err)
	}
	return 0
}

func joinOr(s []string) string {
	if len(s) == 0 {
		return ui.Dim("(none)")
	}
	return strings.Join(s, ", ")
}

func humanBytes(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f kB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
	}
}

// exitCodeFor: lint failures should fail CI only on hard problems.
func exitCodeFor(score int, findings int) int {
	if score < 60 {
		return 1
	}
	return 0
}
