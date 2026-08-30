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

// ForkEdge records that Branch appears to have been cut from From, at the
// commit and time of their inferred common ancestor. This is a heuristic —
// git records no parent-branch pointer — so a ForkEdge is only ever a best
// guess: see inferForkEdges for how it's derived, and AGENTS.md's domain
// notes for why this can't be a fact. Branch never repeats across edges and
// From is never "" — when no candidate parent can be found at all, no edge
// is produced for that branch, rather than guessing.
type ForkEdge struct {
	Branch string    `json:"branch"`
	From   string    `json:"from"`
	Commit string    `json:"commit"`
	At     time.Time `json:"at"`
}

// inferForkEdges guesses, for every non-default branch, which other branch
// it was most likely cut from. Git has no record of this, so it is a
// heuristic in two layers, strongest signal first.
//
// Layer 1 — the merge a branch left behind. If branch C was merged into
// branch P (a MergeEdge with From=C, Into=P), the overwhelmingly common
// workflow is that C was cut from P and later merged back — so C's parent is
// P, and its fork point is merge-base(the merge commit's first parent, C's
// tip): the first parent is P's state at merge time, so a P that has since
// moved on does not drag the fork point forward. This is trusted over
// layer 2, because an observed merge is real topology, not a guess. When C
// merged into several branches, its earliest merge wins.
//
// Layer 2 — pairwise merge-base, for branches with no such merge. Compute
// merge-base(branch, candidate) for every other candidate and pick whichever
// candidate's merge-base is furthest downstream on the branch's own ancestry
// chain (every merge-base(X, *) lies on X's ancestry, so the candidates are
// ancestor-ordered and "furthest downstream" means "least history walked
// back", i.e. the closest relationship). A candidate `other` is only allowed
// to be branch `b`'s parent when canBeParent(other, b) holds:
//
//   - other was merged into b (a layer-1 fact): other joined back into b, it
//     isn't what b grew out of. Disallowed.
//   - merge-base(other, b) == b's tip: b is contained in other, so other is
//     downstream of b, not its origin. Disallowed.
//   - merge-base(other, b) == other's tip: other is a clean, unmoved
//     ancestor of b — the textbook parent. Allowed.
//   - otherwise both have advanced past the shared point: it's a
//     parent/child or a sibling pair, symmetric under merge-base alone.
//     Break it toward the branch that has received merges (an integration
//     branch outranks a leaf), then the more recently active tip, then name
//     order — a total order, so exactly one direction of the pair is
//     allowed and the result is stable without iterating.
//
// The default branch is never assigned a parent: it is the tree's root
// regardless of what its own history looks like.
//
// Cost is O(n^2) git calls for n branches (merge-base memoized, since it's
// symmetric) — fine for the small-to-medium branch counts this has been
// tested against, but worth revisiting per AGENTS.md's scale notes on a
// repository with hundreds of branches.
func inferForkEdges(repoPath string, branches []Branch, merges []MergeEdge) ([]ForkEdge, error) {
	if len(branches) < 2 {
		return nil, nil
	}

	byName := make(map[string]Branch, len(branches))
	for _, b := range branches {
		byName[b.Name] = b
	}

	// mergedInto[C][P] — C was merged into P, so P is downstream-joined from C
	// and cannot be C's fork parent. mergeTarget[P] — P received at least one
	// merge, a weak "this is an integration branch" hint used only to break
	// otherwise-symmetric merge-base ties. firstMerge[C] — C's earliest merge,
	// the layer-1 fork signal.
	type mergeRec struct {
		into   string
		commit string
		when   time.Time
	}
	mergedInto := make(map[string]map[string]bool)
	mergeTarget := make(map[string]bool)
	firstMerge := make(map[string]mergeRec)
	for _, e := range merges {
		mergeTarget[e.Into] = true
		if e.From == "" || e.From == e.Into {
			continue
		}
		if mergedInto[e.From] == nil {
			mergedInto[e.From] = make(map[string]bool)
		}
		mergedInto[e.From][e.Into] = true
		if rec, ok := firstMerge[e.From]; !ok || e.When.Before(rec.when) {
			firstMerge[e.From] = mergeRec{into: e.Into, commit: e.Commit, when: e.When}
		}
	}

	type pairKey struct{ a, b string }
	baseCache := make(map[pairKey]string)
	mergeBase := func(a, b string) (string, bool) {
		key := pairKey{a, b}
		if key.a > key.b {
			key.a, key.b = key.b, key.a
		}
		if base, ok := baseCache[key]; ok {
			return base, base != ""
		}
		out, err := runGit(repoPath, "merge-base", "origin/"+key.a, "origin/"+key.b)
		if err != nil {
			baseCache[key] = ""
			return "", false
		}
		baseCache[key] = out
		return out, true
	}

	resolved := make(map[string]ForkEdge)

	// Layer 1: a branch's own merge tells us where it was cut from.
	for name, rec := range firstMerge {
		b, ok := byName[name]
		if !ok || b.IsDefault || rec.into == name {
			continue
		}
		firstParent, err := runGit(repoPath, "rev-parse", rec.commit+"^1")
		if err != nil {
			continue
		}
		base, err := runGit(repoPath, "merge-base", firstParent, b.Head)
		if err != nil {
			continue
		}
		when, ok := commitDate(repoPath, base)
		if !ok {
			continue
		}
		resolved[name] = ForkEdge{Branch: name, From: rec.into, Commit: base, At: when}
	}

	// canBeParent reports whether p is an allowed fork parent for c: p sits
	// upstream of c, not downstream (merged back in) and not junior to it in
	// the both-advanced case. See the doc comment for the four cases.
	canBeParent := func(p, c Branch) bool {
		if mergedInto[p.Name][c.Name] {
			return false
		}
		base, ok := mergeBase(p.Name, c.Name)
		if !ok || base == c.Head {
			return false
		}
		if base == p.Head {
			return true
		}
		return forkParentPreferred(p, c, mergeTarget)
	}

	// Layer 2: pairwise merge-base for everything layer 1 didn't settle.
	for _, b := range branches {
		if b.IsDefault {
			continue
		}
		if _, done := resolved[b.Name]; done {
			continue // owned by layer 1
		}

		var best Branch
		var bestCommit string
		found := false
		for _, other := range branches {
			if other.Name == b.Name || !canBeParent(other, b) {
				continue
			}
			base, _ := mergeBase(b.Name, other.Name)
			switch {
			case !found:
				best, bestCommit, found = other, base, true
			case base != bestCommit && commitIsAncestor(repoPath, bestCommit, base):
				// base is strictly downstream of the current best — a
				// closer relationship, so other supersedes it.
				best, bestCommit = other, base
			case base == bestCommit && forkParentPreferred(other, best, mergeTarget):
				best = other
			}
		}
		if !found {
			continue
		}
		when, ok := commitDate(repoPath, bestCommit)
		if !ok {
			continue
		}
		resolved[b.Name] = ForkEdge{Branch: b.Name, From: best.Name, Commit: bestCommit, At: when}
	}

	var edges []ForkEdge
	for _, b := range branches {
		if e, ok := resolved[b.Name]; ok {
			edges = append(edges, e)
		}
	}
	return edges, nil
}

// forkParentPreferred reports whether a outranks b as the more parent-like of
// the two when a merge-base alone can't order them — the both-advanced case,
// where a parent that has moved past the fork point and a child (or a sibling)
// look identical. It's a total order: a branch that has received merges
// (likelier an integration branch than a leaf) outranks one that hasn't, then
// the more recently active tip wins, then name order — so exactly one
// direction of any pair is preferred and the result needs no iteration.
func forkParentPreferred(a, b Branch, mergeTarget map[string]bool) bool {
	if mergeTarget[a.Name] != mergeTarget[b.Name] {
		return mergeTarget[a.Name]
	}
	if !a.CommitterDate.Equal(b.CommitterDate) {
		return a.CommitterDate.After(b.CommitterDate)
	}
	return a.Name < b.Name
}

// commitIsAncestor reports whether commit a is an ancestor of (or equal to)
// commit b. `--is-ancestor` exits 1 for "no", which is a normal result, not
// a failure — only an unexpected error (e.g. a bad ref) collapses to false,
// the conservative "don't treat this as more specific than it is" default.
func commitIsAncestor(repoPath, a, b string) bool {
	cmd := exec.Command("git", "-C", repoPath, "merge-base", "--is-ancestor", a, b)
	return cmd.Run() == nil
}

// commitDate is the committer date of an arbitrary commit (not necessarily a
// branch tip).
func commitDate(repoPath, commit string) (time.Time, bool) {
	out, err := runGit(repoPath, "show", "-s", "--format=%cI", commit)
	if err != nil {
		return time.Time{}, false
	}
	when, err := time.Parse(time.RFC3339, out)
	return when, err == nil
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
