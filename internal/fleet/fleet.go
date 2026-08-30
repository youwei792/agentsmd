// Package fleet implements `agentsmd org`: scan every repository of a
// GitHub user or organization and report the health of its AGENTS.md
// files. Fleet-level adoption is the point: a team that manages N repos
// with agentsmd does not switch tooling per repo.
//
// The fetcher shells out to the gh CLI (already required for GitHub auth)
// so the Go binary itself keeps zero third-party dependencies.
package fleet

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/youwei792/agentsmd/internal/tokens"
)

// RepoReport is the AGENTS.md health of one repository.
type RepoReport struct {
	Repo      string `json:"repo"`
	HasAgents bool   `json:"has_agents"`
	Tokens    int    `json:"tokens"`
	Bytes     int    `json:"bytes"`
	Sections  int    `json:"sections"`
	CodeRefs  int    `json:"code_refs"` // fenced code blocks (commands documented?)
	TODOs     int    `json:"todos"`
}

// FleetReport aggregates per-repo results.
type FleetReport struct {
	Owner      string       `json:"owner"`
	ReposSeen  int          `json:"repos_seen"`  // total public repos encountered
	ReposHit   int          `json:"repos_hit"`   // those with a root AGENTS.md
	Reports    []RepoReport `json:"reports"`     // only the hits, sorted by tokens desc
	TotalTokens int         `json:"total_tokens"`
}

// Scan returns the fleet report for owner, examining at most limit repos.
// ghCommand is the gh binary to use (allows tests to stub it).
func Scan(ghCommand, owner string, limit int) (*FleetReport, error) {
	if _, err := exec.LookPath(ghCommand); err != nil {
		return nil, fmt.Errorf("agentsmd org requires the GitHub CLI (gh) — install it from https://cli.github.com")
	}

	names, err := listRepos(ghCommand, owner)
	if err != nil {
		return nil, err
	}
	if len(names) > limit {
		names = names[:limit]
	}

	rep := &FleetReport{Owner: owner, ReposSeen: len(names)}
	for _, full := range names {
		content, ok := fetchAgentsMd(ghCommand, full)
		if !ok {
			continue
		}
		r := RepoReport{
			Repo:      full,
			HasAgents: true,
			Bytes:     len(content),
			Tokens:    tokens.Estimate(content),
			Sections:  strings.Count(content, "\n## "),
			CodeRefs:  strings.Count(content, "```") / 2,
			TODOs:     strings.Count(strings.ToUpper(content), "TODO") + strings.Count(strings.ToUpper(content), "TBD"),
		}
		rep.Reports = append(rep.Reports, r)
		rep.ReposHit++
		rep.TotalTokens += r.Tokens
	}
	sort.Slice(rep.Reports, func(i, j int) bool {
		if rep.Reports[i].Tokens != rep.Reports[j].Tokens {
			return rep.Reports[i].Tokens > rep.Reports[j].Tokens
		}
		return rep.Reports[i].Repo < rep.Reports[j].Repo
	})
	return rep, nil
}

// listRepos returns full_name for each public repo of the owner.
func listRepos(gh, owner string) ([]string, error) {
	// Organizations and users expose different endpoints; try orgs first.
	for _, ep := range []string{"orgs/" + owner + "/repos?per_page=100", "users/" + owner + "/repos?per_page=100"} {
		out, err := runGH(gh, ep)
		if err != nil {
			continue
		}
		var nodes []struct {
			FullName string `json:"full_name"`
			Fork     bool   `json:"fork"`
		}
		if json.Unmarshal(out, &nodes) != nil {
			continue
		}
		var names []string
		for _, n := range nodes {
			if !n.Fork && n.FullName != "" {
				names = append(names, n.FullName)
			}
		}
		if len(nodes) > 0 {
			return names, nil
		}
	}
	return nil, fmt.Errorf("could not list repositories for %q (does the account/org exist, and are its repos public?)", owner)
}

// fetchAgentsMd fetches the root AGENTS.md of a repo via the contents API.
func fetchAgentsMd(gh, full string) (string, bool) {
	out, err := runGH(gh, "repos/"+full+"/contents/AGENTS.md")
	if err != nil {
		return "", false
	}
	var node struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if json.Unmarshal(out, &node) != nil || node.Encoding != "base64" {
		return "", false
	}
	b, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(node.Content, "\n", ""))
	if err != nil {
		return "", false
	}
	return string(b), true
}

func runGH(gh, endpoint string) ([]byte, error) {
	cmd := exec.Command(gh, "api", endpoint)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh api %s: %v: %s", endpoint, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
