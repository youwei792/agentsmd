// agentsmd is a single-binary toolkit for the agent-instruction files that
// AI coding agents read: AGENTS.md, CLAUDE.md, GEMINI.md and friends.
//
// It analyzes a repository, generates a grounded AGENTS.md, validates that
// every command and file the document mentions actually exists, audits
// quality and token cost, and keeps tool-specific files in sync.
package main

import (
	"os"

	"github.com/youwei792/agentsmd/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
