package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Branch is a single local branch and its merge status, read live from the
// repository — never persisted.
type Branch struct {
	Name          string    `json:"name"`
	Head          string    `json:"head"`
	IsCurrent     bool      `json:"isCurrent"`
	CommitterDate time.Time `json:"committerDate"`
	// MergedToHead reports that this branch's tip is an ancestor of HEAD, i.e.
	// it has already been merged.
	MergedToHead bool `json:"mergedToHead"`
}

// MergeEdge is one merge commit found by walking a branch's first-parent chain:
// Into received the merge. From is the branch whose tip was merged in, filled in
// only when the merged-in commit exactly matches a currently existing local
// branch's tip — if that branch has since been deleted, From is "" rather than a
// guess (see AGENTS.md: never silently guess wrong about topology).
type MergeEdge struct {
	Into   string    `json:"into"`
	From   string    `json:"from"`
	Commit string    `json:"commit"`
	When   time.Time `json:"when"`
}

// readBranches lists repoPath's local branches with their merge status relative
// to HEAD. An unborn HEAD (freshly `git init`ed, no commits yet) is not an
// error — it just yields no branches.
func readBranches(repoPath string) ([]Branch, error) {
	out, err := runGit(repoPath, "for-each-ref",
		"--format=%(HEAD)%00%(refname:short)%00%(objectname)%00%(committerdate:iso-strict)",
		"refs/heads")
	if err != nil {
		return nil, fmt.Errorf("listing branches: %w", err)
	}

	var branches []Branch
	for _, line := range splitLines(out) {
		fields := strings.Split(line, "\x00")
		if len(fields) != 4 {
			return nil, fmt.Errorf("listing branches: unexpected for-each-ref output %q", line)
		}
		when, err := time.Parse(time.RFC3339, fields[3])
		if err != nil {
			return nil, fmt.Errorf("listing branches: parsing committer date %q: %w", fields[3], err)
		}
		branches = append(branches, Branch{
			Name:          fields[1],
			Head:          fields[2],
			IsCurrent:     fields[0] == "*",
			CommitterDate: when,
		})
	}
	if len(branches) == 0 {
		// `git branch --merged` resolves HEAD, which fails on an unborn HEAD
		// (no commits yet) — there is nothing to merge-check anyway.
		return nil, nil
	}

	merged, err := runGit(repoPath, "branch", "--merged")
	if err != nil {
		return nil, fmt.Errorf("listing merged branches: %w", err)
	}
	mergedNames := make(map[string]bool)
	for _, line := range splitLines(merged) {
		mergedNames[strings.TrimLeft(line, "* +")] = true
	}

	for i := range branches {
		branches[i].MergedToHead = mergedNames[branches[i].Name]
	}
	return branches, nil
}

// readMergeEdges walks each branch's first-parent chain looking for merge
// commits, matching the domain note: "its first parent is the branch that
// received the merge, the others are what was merged in." branches supplies the
// tip->name map used to resolve a merge edge's source branch.
func readMergeEdges(repoPath string, branches []Branch) ([]MergeEdge, error) {
	tipNames := make(map[string]string, len(branches))
	for _, b := range branches {
		tipNames[b.Head] = b.Name
	}

	var edges []MergeEdge
	for _, b := range branches {
		out, err := runGit(repoPath, "log", "--first-parent", "--merges",
			"--format=%H%x00%P%x00%cI", b.Name)
		if err != nil {
			return nil, fmt.Errorf("walking merge history of %s: %w", b.Name, err)
		}

		for _, line := range splitLines(out) {
			fields := strings.Split(line, "\x00")
			if len(fields) != 3 {
				return nil, fmt.Errorf("walking merge history of %s: unexpected log output %q", b.Name, line)
			}
			commit, parents, when := fields[0], strings.Fields(fields[1]), fields[2]
			mergedAt, err := time.Parse(time.RFC3339, when)
			if err != nil {
				return nil, fmt.Errorf("walking merge history of %s: parsing commit date %q: %w", b.Name, when, err)
			}

			for _, parent := range parents[1:] {
				from := tipNames[parent] // "" if the source branch no longer exists
				if from == b.Name {
					continue
				}
				edges = append(edges, MergeEdge{
					Into:   b.Name,
					From:   from,
					Commit: commit,
					When:   mergedAt,
				})
			}
		}
	}
	return edges, nil
}

// runGit runs git in dir and returns its trimmed standard output. Failures fold
// standard error into the returned error so they are diagnosable — a missing
// git binary or a repository in a bad state should be obvious from the message
// alone.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// splitLines splits git's line-oriented output, returning no lines for empty
// input rather than a single empty one.
func splitLines(out string) []string {
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}
