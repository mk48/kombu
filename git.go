package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Branch is a single branch on the "origin" remote and its merge status, read
// live from the repository's local refs/remotes/origin/* cache — never
// persisted. Names have the "origin/" prefix stripped (e.g. "main", not
// "origin/main").
type Branch struct {
	Name          string    `json:"name"`
	Head          string    `json:"head"`
	CommitterDate time.Time `json:"committerDate"`
	// IsCurrent reports that this is the upstream of the repository's currently
	// checked-out local branch (by name — not a strict tracking-ref check).
	// False, rather than a guess, when HEAD is detached or has no same-named
	// origin branch.
	IsCurrent bool `json:"isCurrent"`
	// IsDefault reports that this is origin's default branch, resolved from the
	// refs/remotes/origin/HEAD symref. That symref isn't always present (e.g. a
	// manually added remote), in which case no branch is marked default.
	IsDefault bool `json:"isDefault"`
	// MergedToDefault reports that this branch's tip is an ancestor of origin's
	// default branch, i.e. it has already been merged there. Always false when
	// IsDefault couldn't be resolved for any branch.
	MergedToDefault bool `json:"mergedToDefault"`
}

// MergeEdge is one merge commit found by walking an origin branch's
// first-parent chain: Into received the merge. From is the origin branch whose
// tip was merged in, filled in only when the merged-in commit exactly matches
// another current origin branch's tip — if that branch has since been deleted
// on the server, From is "" rather than a guess (see AGENTS.md: never silently
// guess wrong about topology).
type MergeEdge struct {
	Into   string    `json:"into"`
	From   string    `json:"from"`
	Commit string    `json:"commit"`
	When   time.Time `json:"when"`
}

// readBranches lists repoPath's branches on the "origin" remote — the server
// side, not the local checkout — with their merge status relative to origin's
// default branch. No commits pushed to origin yet, or no "origin" remote at
// all, is not an error — it just yields no branches. This reads whatever
// refs/remotes/origin/* already holds; it does not fetch.
func readBranches(repoPath string) ([]Branch, error) {
	out, err := runGit(repoPath, "for-each-ref",
		"--format=%(refname:short)%00%(objectname)%00%(committerdate:iso-strict)",
		"refs/remotes/origin")
	if err != nil {
		return nil, fmt.Errorf("listing origin branches: %w", err)
	}

	var branches []Branch
	for _, line := range splitLines(out) {
		fields := strings.Split(line, "\x00")
		if len(fields) != 3 {
			return nil, fmt.Errorf("listing origin branches: unexpected for-each-ref output %q", line)
		}
		// refs/remotes/origin/HEAD's short form is "origin" (no slash) — it is
		// an alias for origin's default branch, not a branch of its own.
		name, ok := strings.CutPrefix(fields[0], "origin/")
		if !ok {
			continue
		}
		when, err := time.Parse(time.RFC3339, fields[2])
		if err != nil {
			return nil, fmt.Errorf("listing origin branches: parsing committer date %q: %w", fields[2], err)
		}
		branches = append(branches, Branch{
			Name:          name,
			Head:          fields[1],
			CommitterDate: when,
		})
	}
	if len(branches) == 0 {
		return nil, nil
	}

	current, haveCurrent := currentBranchName(repoPath)
	defaultBranch, haveDefault := originDefaultBranch(repoPath)

	var mergedNames map[string]bool
	if haveDefault {
		merged, err := runGit(repoPath, "branch", "--remotes", "--merged", "origin/"+defaultBranch)
		if err != nil {
			return nil, fmt.Errorf("listing branches merged into origin/%s: %w", defaultBranch, err)
		}
		mergedNames = make(map[string]bool)
		for _, line := range splitLines(merged) {
			line = strings.TrimSpace(line)
			// `branch --remotes` includes a synthetic "origin/HEAD -> origin/main"
			// line alongside real branches; it is not a branch to record.
			if line == "" || strings.Contains(line, "->") {
				continue
			}
			if name, ok := strings.CutPrefix(line, "origin/"); ok {
				mergedNames[name] = true
			}
		}
	}

	for i := range branches {
		branches[i].IsCurrent = haveCurrent && branches[i].Name == current
		branches[i].IsDefault = haveDefault && branches[i].Name == defaultBranch
		branches[i].MergedToDefault = mergedNames[branches[i].Name]
	}
	return branches, nil
}

// currentBranchName is the name of the local checkout's current branch, used
// only to highlight the matching origin branch, if any. ok is false for a
// detached HEAD or an unborn one — neither is an error, just nothing to mark.
func currentBranchName(repoPath string) (name string, ok bool) {
	out, err := runGit(repoPath, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return "", false
	}
	return out, true
}

// originDefaultBranch resolves origin's default branch (e.g. "main") from the
// refs/remotes/origin/HEAD symref, which a normal clone sets from the remote's
// own default at clone time. It is not always present — a remote added by hand,
// or one whose default changed since — so a resolution failure is not an error,
// just an unresolved default that leaves IsDefault and MergedToDefault unset.
func originDefaultBranch(repoPath string) (name string, ok bool) {
	out, err := runGit(repoPath, "symbolic-ref", "--short", "-q", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", false
	}
	name, ok = strings.CutPrefix(out, "origin/")
	return name, ok
}

// readMergeEdges walks each origin branch's first-parent chain looking for
// merge commits, matching the domain note: "its first parent is the branch
// that received the merge, the others are what was merged in." branches
// supplies the tip->name map used to resolve a merge edge's source branch.
//
// A branch's first-parent chain runs all the way back to the repository's
// root, so a branch forked from the default branch inherits every merge in
// the default branch's history too — walked from the feature branch's tip,
// those commits look identical to a merge "into" the feature branch, even
// though the feature branch didn't exist yet when they happened. Those are
// filtered out by filterInheritedMerges before returning: only the default
// branch keeps its full history, and every other branch keeps only merge
// commits that are not already part of it. This does not resolve the same
// duplication between two branches that both fork from a shared non-default
// ancestor — a known limitation, not a full ownership computation.
func readMergeEdges(repoPath string, branches []Branch) ([]MergeEdge, error) {
	tipNames := make(map[string]string, len(branches))
	var defaultBranch string
	for _, b := range branches {
		tipNames[b.Head] = b.Name
		if b.IsDefault {
			defaultBranch = b.Name
		}
	}

	perBranch := make(map[string][]MergeEdge, len(branches))
	for _, b := range branches {
		out, err := runGit(repoPath, "log", "--first-parent", "--merges",
			"--format=%H%x00%P%x00%cI", "origin/"+b.Name)
		if err != nil {
			return nil, fmt.Errorf("walking merge history of %s: %w", b.Name, err)
		}

		var edges []MergeEdge
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
		perBranch[b.Name] = edges
	}

	return filterInheritedMerges(perBranch, defaultBranch), nil
}

// filterInheritedMerges drops, from every branch but defaultBranch, any
// merge commit that defaultBranch's own history already claims — see
// readMergeEdges for why those show up in the first place. defaultBranch's
// own edges are returned untouched (there's no more-upstream branch to defer
// to); if it couldn't be resolved, nothing is filtered.
func filterInheritedMerges(perBranch map[string][]MergeEdge, defaultBranch string) []MergeEdge {
	trunkCommits := make(map[string]bool, len(perBranch[defaultBranch]))
	for _, edge := range perBranch[defaultBranch] {
		trunkCommits[edge.Commit] = true
	}

	var edges []MergeEdge
	for branch, branchEdges := range perBranch {
		for _, edge := range branchEdges {
			if branch != defaultBranch && trunkCommits[edge.Commit] {
				continue
			}
			edges = append(edges, edge)
		}
	}
	return edges
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
