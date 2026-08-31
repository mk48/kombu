# From `git` commands to the branch graph

A tutorial on how kombu turns a repository's refs into the graph the lane view
draws. Everything here lives in [git.go](../git.go); the frontend consumes the
result but does no Git work of its own.

kombu never runs `git fetch`. It reads whatever `refs/remotes/origin/*` already
holds in the local checkout, so the picture is exactly as fresh as the user's
last manual fetch.

---

## 1. The graph we are building

The graph is **not** a commit graph. Its nodes are **branches** and its edges are
**relationships between branches**:

| Graph element | Go type | Meaning |
|---|---|---|
| Node | `Branch` | one branch on `origin`, plus its merge status |
| Edge (observed) | `MergeEdge` | branch *From* was merged **into** branch *Into* |
| Edge (inferred) | `ForkEdge` | branch *Branch* was most likely **cut from** branch *From* |

```go
type Branch struct {
	Name          string    // "main", "dev" — the "origin/" prefix stripped
	Head          string    // tip commit SHA
	CommitterDate time.Time // tip commit's committer date
	IsCurrent     bool      // upstream of the local checkout's current branch
	IsDefault     bool      // origin's default branch (from refs/remotes/origin/HEAD)
	MergedToDefault bool    // tip is an ancestor of the default branch
}

type MergeEdge struct {
	Into   string    // branch that received the merge
	From   string    // branch that was merged in ("" if it no longer exists on origin)
	Commit string    // the merge commit SHA
	When   time.Time // merge commit's committer date
}

type ForkEdge struct {
	Branch string    // the child branch
	From   string    // the inferred parent branch
	Commit string    // the inferred fork-point commit
	At     time.Time // fork-point commit's committer date
}
```

`WorkspaceService.GetBranches` (in [workspaceservice.go](../workspaceservice.go))
runs the pipeline and hands the frontend one `BranchInfo`:

```go
branches, _ := readBranches(repo.Path)                 // step 2–4
merges,   _ := readMergeEdges(repo.Path, branches)     // step 5
forks,    _ := inferForkEdges(repo.Path, branches, merges) // step 6
return BranchInfo{
	Branches:  branches,
	Merges:    merges,
	Forks:     forks,
	LaneOrder: reconcileLaneOrder(repo.LaneOrder, branches), // step 7
}
```

Every shell-out goes through one helper:

```go
// runGit runs `git -C <dir> <args...>` and returns trimmed stdout.
// On failure it folds stderr into the error so a broken repo is diagnosable.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	// ... capture stdout/stderr, cmd.Run() ...
}
```

All output is requested in a machine-readable form — explicit `--format`
strings, NUL (`%00` / `\x00`) field separators, ISO-8601 dates — never parsed
from porcelain text.

---

## A running example

Every sample output below comes from this repository:

```
main     ●──●──────────────●        (default branch)
              \           /
dev            ●──●──────●──●        merge commit M closes feature1
                   \    /
feature1            ●──●             merged back into dev via M
                   \
feature2            ●──●             still open
```

- `dev` was cut from `main`.
- `feature1` and `feature2` were both cut from the same commit on `dev` (call it `F`).
- `feature1` was merged back into `dev` (merge commit `M`, message
  `Merge pull request #1 from mk48/feature1`).
- `feature2` is still open.

---

## 2. Branch nodes — `git for-each-ref`

### Command

```
git -C <repo> for-each-ref \
  --format=%(refname:short)%00%(objectname)%00%(committerdate:iso-strict) \
  refs/remotes/origin
```

### Sample output

`%00` is a NUL byte; shown here as `<NUL>`.

```
origin<NUL>a1b2c3d4...<NUL>2026-08-30T09:12:44+00:00
origin/main<NUL>a1b2c3d4e5f6...<NUL>2026-08-30T09:12:44+00:00
origin/dev<NUL>b2c3d4e5f6a7...<NUL>2026-08-31T14:03:20+00:00
origin/feature1<NUL>c3d4e5f6a7b8...<NUL>2026-08-29T11:40:05+00:00
origin/feature2<NUL>d4e5f6a7b8c9...<NUL>2026-08-31T08:55:12+00:00
```

### How it becomes graph nodes

```go
for _, line := range splitLines(out) {
	fields := strings.Split(line, "\x00")        // [shortname, sha, isodate]
	name, ok := strings.CutPrefix(fields[0], "origin/")
	if !ok {
		continue // "origin" with no slash is refs/remotes/origin/HEAD — an alias, not a branch
	}
	when, _ := time.Parse(time.RFC3339, fields[2])
	branches = append(branches, Branch{
		Name:          name,
		Head:          fields[1],
		CommitterDate: when,
	})
}
```

The first line (`origin`, no slash) is `refs/remotes/origin/HEAD`. Its short form
has no `/`, so `CutPrefix` fails and it is skipped — it is a pointer to the
default branch, not a branch of its own.

After this step we have four nodes: `main`, `dev`, `feature1`, `feature2`, each
with a tip SHA and a date, and nothing else filled in.

No `origin` remote, or nothing fetched yet, yields an empty list — not an error.

---

## 3. `IsCurrent` and `IsDefault` — `git symbolic-ref`

Two cheap symref reads decorate the nodes.

### Current local branch

```
git -C <repo> symbolic-ref --short -q HEAD
```

```
dev
```

`-q` makes a detached HEAD exit non-zero instead of erroring loudly; kombu reads
that as "nothing to mark" rather than a failure.

### origin's default branch

```
git -C <repo> symbolic-ref --short -q refs/remotes/origin/HEAD
```

```
origin/main
```

This symref is set at clone time from the remote's default. It is **not** always
present (a remote added by hand, for instance), so a failure here is not an
error — it just leaves `IsDefault` and `MergedToDefault` unset for every branch.

### How it decorates the nodes

```go
current, haveCurrent  := currentBranchName(repoPath)  // "dev", true
defaultBranch, haveDefault := originDefaultBranch(repoPath) // "main", true

for i := range branches {
	branches[i].IsCurrent = haveCurrent && branches[i].Name == current
	branches[i].IsDefault = haveDefault && branches[i].Name == defaultBranch
}
```

Now `main.IsDefault == true` and `dev.IsCurrent == true`.

---

## 4. `MergedToDefault` — `git branch --merged`

The cheap "is this branch already merged into trunk?" answer, used to grey out
closed branches.

### Command

```
git -C <repo> branch --remotes --merged origin/main
```

### Sample output

```
  origin/HEAD -> origin/main
  origin/feature1
  origin/main
```

`feature1` is listed because its tip is an ancestor of `origin/main`'s tip
(reachable by following `dev` and the merge). `dev` and `feature2` are not:
they have commits `main` has never seen.

### How it decorates the nodes

```go
merged, _ := runGit(repoPath, "branch", "--remotes", "--merged", "origin/"+defaultBranch)
mergedNames := map[string]bool{}
for _, line := range splitLines(merged) {
	line = strings.TrimSpace(line)
	if line == "" || strings.Contains(line, "->") {
		continue // skip the synthetic "origin/HEAD -> origin/main" line
	}
	if name, ok := strings.CutPrefix(line, "origin/"); ok {
		mergedNames[name] = true
	}
}
// ...
branches[i].MergedToDefault = mergedNames[branches[i].Name]
```

Result: `feature1.MergedToDefault == true`; `dev` and `feature2` stay false.

At this point `readBranches` returns a complete list of decorated nodes.

---

## 5. Merge edges — `git log --first-parent --merges`

### The key fact

A merge commit has **two or more parents**. Its **first parent** is the branch
that *received* the merge; the **other parents** are what was *merged in*. So
walking a branch tip's first-parent chain and stopping at each merge commit
yields every merge **into** that branch.

### Command (run once per branch)

```
git -C <repo> log --first-parent --merges \
  --format=%H%x00%P%x00%cI \
  origin/dev
```

- `--first-parent` — only follow parent #1 at each merge, so we stay on `dev`.
- `--merges` — only show commits with 2+ parents.
- `%H` commit SHA, `%P` all parent SHAs (space-separated), `%cI` committer date ISO-8601.

### Sample output (for `origin/dev`)

```
M0M0M0...<NUL>P1P1P1... C1C1C1...<NUL>2026-08-31T13:59:00+00:00
```

One merge commit `M`. Its parents are `P1` (the previous tip of `dev`, the first
parent) and `C1` (the tip of `feature1`, the second parent).

Running the same command for `origin/main`, `origin/feature1`, `origin/feature2`
produces no output — none of them contain a merge commit on their first-parent
spine in this example.

### How it becomes graph edges

```go
tipNames := map[string]string{} // SHA -> branch name, for resolving the source
for _, b := range branches {
	tipNames[b.Head] = b.Name
}

for _, b := range branches {
	out, _ := runGit(repoPath, "log", "--first-parent", "--merges",
		"--format=%H%x00%P%x00%cI", "origin/"+b.Name)

	for _, line := range splitLines(out) {
		fields := strings.Split(line, "\x00")
		commit  := fields[0]
		parents := strings.Fields(fields[1]) // [firstParent, merged1, merged2, ...]
		when, _ := time.Parse(time.RFC3339, fields[2])

		for _, parent := range parents[1:] { // skip parents[0] — that's `b` itself
			from := tipNames[parent] // "" if that SHA is no longer any branch's tip
			if from == b.Name {
				continue
			}
			edges = append(edges, MergeEdge{
				Into: b.Name, From: from, Commit: commit, When: when,
			})
		}
	}
}
```

For our example this yields exactly one edge:

```go
MergeEdge{Into: "dev", From: "feature1", Commit: "M0M0M0...", When: 2026-08-31T13:59:00Z}
```

`parents[1:]` handles **octopus merges** (3+ parents) naturally: one `MergeEdge`
per merged-in branch, all sharing `Commit`.

If `feature1` had been deleted from origin after the merge, `tipNames[C1]` would
miss and `From` would be `""` — the edge is still recorded (something *did* land
on `dev`), just without a known source. The frontend draws that as a dashed stub
instead of a curve.

### `filterInheritedMerges` — why a branch "has" merges it never made

A branch's first-parent chain runs all the way back to the root commit. A branch
cut from `main` *after* some feature was merged into `main` will, when you walk
its first-parent chain, pass straight through that old merge commit — even though
the branch did not exist when it happened.

```go
// Keep the default branch's merges as-is. For every other branch, drop any
// merge commit that the default branch's history already claims.
func filterInheritedMerges(perBranch map[string][]MergeEdge, defaultBranch string) []MergeEdge {
	trunkCommits := map[string]bool{}
	for _, edge := range perBranch[defaultBranch] {
		trunkCommits[edge.Commit] = true
	}

	var edges []MergeEdge
	for branch, branchEdges := range perBranch {
		for _, edge := range branchEdges {
			if branch != defaultBranch && trunkCommits[edge.Commit] {
				continue // inherited from trunk, not this branch's own topology
			}
			edges = append(edges, edge)
		}
	}
	return edges
}
```

This matters at scale: without it, every feature branch in a busy repo would
appear to have "merged in" the entire release history of trunk.

---

## 6. Fork edges — `git merge-base`, `rev-parse`, `git show`

**Git records no parent-branch pointer.** "Which branch was this cut from?" is a
*heuristic*, and the fork edges are drawn dashed with a hollow arrowhead to say
so. `inferForkEdges` works in two layers, strongest signal first.

### Layer 1 — the merge a branch left behind

If branch `C` was merged **into** branch `P` (we observed that in step 5), then
overwhelmingly `C` was cut from `P` and later merged back. That is an *observed*
merge, not a guess, so it wins.

The fork point is `merge-base(first-parent-of-the-merge-commit, C's tip)`. Using
the merge commit's first parent — `P`'s state *at merge time* — means a `P` that
has moved on since does not drag the fork point forward.

```go
// firstMerge[C] = C's earliest observed merge, built from the MergeEdges
for name, rec := range firstMerge {          // rec.into = P, rec.commit = M
	b := byName[name]                        // b = C
	if b.IsDefault || rec.into == name {
		continue
	}
	firstParent, _ := runGit(repoPath, "rev-parse", rec.commit+"^1")
	base, _        := runGit(repoPath, "merge-base", firstParent, b.Head)
	when, _        := commitDate(repoPath, base)
	resolved[name] = ForkEdge{Branch: name, From: rec.into, Commit: base, At: when}
}
```

#### Sample commands and output

```
$ git -C <repo> rev-parse M0M0M0...^1
P1P1P1...

$ git -C <repo> merge-base P1P1P1... c3d4e5f6a7b8...   # first parent vs feature1 tip
F0F0F0...                                              # commit F on dev

$ git -C <repo> show -s --format=%cI F0F0F0...
2026-08-29T10:15:00+00:00
```

Giving:

```go
ForkEdge{Branch: "feature1", From: "dev", Commit: "F0F0F0...", At: 2026-08-29T10:15:00Z}
```

### Layer 2 — pairwise `merge-base` for everything else

`dev` and `feature2` have no merge of their own, so we fall back to computing
`merge-base(branch, candidate)` against every other branch and picking the best
candidate.

```
$ git -C <repo> merge-base origin/dev origin/main
A0A0A0...        # the commit on main where dev started

$ git -C <repo> merge-base origin/feature2 origin/dev
F0F0F0...        # commit F — same fork point as feature1

$ git -C <repo> merge-base origin/feature2 origin/main
A0A0A0...        # further back — a worse (more distant) candidate
```

Every `merge-base(X, *)` lies on `X`'s own ancestry, so the candidates are
ancestor-ordered. "Best" = **furthest downstream** on the branch's ancestry =
least history walked back = closest relationship. For `feature2`, `F` (shared
with `dev`) beats `A` (shared with `main`), so the parent is `dev`.

```go
for _, other := range branches {
	if other.Name == b.Name || !canBeParent(other, b) {
		continue
	}
	base, _ := mergeBase(b.Name, other.Name)
	switch {
	case !found:
		best, bestCommit, found = other, base, true
	case base != bestCommit && commitIsAncestor(repoPath, bestCommit, base):
		best, bestCommit = other, base            // `base` is strictly downstream — closer
	case base == bestCommit && forkParentPreferred(other, best, mergeTarget):
		best = other                              // tie — break toward the integration branch
	}
}
```

#### `canBeParent` — getting the *direction* right

The subtle bug this guards against: a feature branch that was **merged back** has
`merge-base(feature, parent) == feature's own tip`, which naively reads as "the
parent was cut from the feature". `canBeParent(p, c)` rejects that:

```go
func canBeParent(p, c Branch) bool {
	if mergedInto[p.Name][c.Name] {
		return false // p merged into c — p joined back into c, isn't what c grew from
	}
	base, ok := mergeBase(p.Name, c.Name)
	if !ok || base == c.Head {
		return false // c is fully contained in p — p is downstream of c
	}
	if base == p.Head {
		return true  // p is a clean, unmoved ancestor of c — the textbook parent
	}
	return forkParentPreferred(p, c, mergeTarget) // both advanced — use the tie-break order
}
```

`forkParentPreferred` is a total order: a branch that has *received* merges (more
likely an integration branch) outranks a leaf, then the more recently active
tip, then name order. Being total, exactly one direction of any pair is allowed
and the result is stable without iteration.

The default branch is **never** assigned a parent — it is the root of the tree
regardless of what its own history looks like. A branch with no confident
candidate (e.g. unrelated/orphan history) gets **no edge** rather than a guess.

### Result for the example

```go
[]ForkEdge{
	{Branch: "dev",      From: "main", Commit: "A0A0A0...", At: ...}, // layer 2
	{Branch: "feature1", From: "dev",  Commit: "F0F0F0...", At: ...}, // layer 1
	{Branch: "feature2", From: "dev",  Commit: "F0F0F0...", At: ...}, // layer 2
}
```

Cost is `O(n²)` `merge-base` calls for `n` branches (memoized, since `merge-base`
is symmetric) — fine for the branch counts kombu has been tested against.

---

## 7. Lane order — `reconcileLaneOrder`

No Git here — this is the last step in [lanes.go](../lanes.go). Lane assignment
is an *ordering* problem, not a packing problem: every branch gets exactly one
lane, so the only question is the vertical order.

```go
func reconcileLaneOrder(saved []string, branches []Branch) []string {
	// 1. names from the user's saved drag order that still match a live branch,
	//    in their saved relative position
	// 2. then every remaining branch: default branch first,
	//    then by CommitterDate descending
}
```

The saved order is `Repo.LaneOrder`, persisted per repository whenever the user
drags a lane row. Reconciling against the freshly-read branch list means a
deleted branch drops out silently and a new branch appends in a sensible spot,
so the picture never reshuffles wholesale on refresh.

---

## 8. From structs to pixels

The frontend (`frontend/src/components/branch-tree/`) does no Git work. It
receives `BranchInfo` and maps it straight onto SVG geometry:

| Data | Drawing |
|---|---|
| `LaneOrder` | `laneIndex: Map<branchName, rowIndex>` — the Y position of every lane |
| `Branch.committerDate`, `MergeEdge.when`, `ForkEdge.at` | fed into a shared d3 `scaleTime` → the X axis (`time-scale.ts`) |
| `Branch` | one horizontal line per lane, spanning only its known timestamps (`lane-bars.tsx`) |
| `MergeEdge` | **solid** curve from `laneY(from)` to `laneY(into)`, landing at `when`, filled arrowhead (`merge-connectors.tsx`) |
| `MergeEdge` with `From: ""` | short dashed stub — something landed, source unknown |
| `ForkEdge` | **dashed** curve from `laneY(from)` to `laneY(branch)` at `at`, hollow arrowhead — inferred, not observed (`fork-connectors.tsx`) |

Node identity is the branch name; `laneY(index) = index * LANE_HEIGHT +
LANE_HEIGHT/2`. Because one shared `<svg>` holds every bar and every connector in
that single coordinate space, a connector between two far-apart lanes — one of
them scrolled off-screen — is correct by construction, with no scroll-sync code.

---

## Command reference

| Command | Purpose | Produces |
|---|---|---|
| `for-each-ref --format=... refs/remotes/origin` | enumerate origin branch tips | `Branch` nodes |
| `symbolic-ref --short -q HEAD` | local current branch | `Branch.IsCurrent` |
| `symbolic-ref --short -q refs/remotes/origin/HEAD` | origin's default branch | `Branch.IsDefault` |
| `branch --remotes --merged origin/<default>` | already-merged branches | `Branch.MergedToDefault` |
| `log --first-parent --merges --format=%H%x00%P%x00%cI origin/<b>` | merges into branch `b` | `MergeEdge` edges |
| `rev-parse <merge>^1` | merge commit's first parent | fork-point input (layer 1) |
| `merge-base <a> <b>` | best common ancestor | `ForkEdge.Commit` |
| `merge-base --is-ancestor <a> <b>` | ancestry test (exit code) | layer-2 "closer candidate" check |
| `show -s --format=%cI <commit>` | committer date of any commit | `ForkEdge.At` |

## Where to read the code

- [git.go](../git.go) — every command above, and the parsing
- [git_test.go](../git_test.go) — fixtures that build these exact topologies with real `git` calls
- [workspaceservice.go](../workspaceservice.go) — `GetBranches` wires the pipeline together
- [lanes.go](../lanes.go) — lane ordering
- `frontend/src/components/branch-tree/` — the SVG renderer
