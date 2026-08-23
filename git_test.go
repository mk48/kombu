package main

import (
	"path/filepath"
	"testing"
)

// newOriginFixture creates a bare "server" repository and a clone of it. Tests
// exercise readBranches/readMergeEdges against the clone's directory, matching
// how kombu reads a real checkout's refs/remotes/origin/* cache.
func newOriginFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bare := filepath.Join(dir, "origin.git")
	clone := filepath.Join(dir, "clone")

	mustGit(t, dir, "init", "--bare", "-q", "-b", "main", bare)
	mustGit(t, dir, "clone", "-q", bare, clone)
	mustGit(t, clone, "config", "user.email", "test@example.com")
	mustGit(t, clone, "config", "user.name", "Test")
	return clone
}

// commit makes an empty commit so fixtures don't need real file content.
func commit(t *testing.T, dir, message string) string {
	t.Helper()
	mustGit(t, dir, "commit", "--allow-empty", "-q", "-m", message)
	return mustGit(t, dir, "rev-parse", "HEAD")
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runGit(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func branchByName(branches []Branch, name string) (Branch, bool) {
	for _, b := range branches {
		if b.Name == name {
			return b, true
		}
	}
	return Branch{}, false
}

func TestReadBranches(t *testing.T) {
	clone := newOriginFixture(t)
	commit(t, clone, "initial")
	mustGit(t, clone, "push", "-q", "origin", "HEAD:main")

	mustGit(t, clone, "checkout", "-q", "-b", "merged-feature")
	commit(t, clone, "feature work")
	mustGit(t, clone, "push", "-q", "origin", "merged-feature")

	mustGit(t, clone, "checkout", "-q", "main")
	mustGit(t, clone, "merge", "-q", "--no-ff", "-m", "merge feature", "merged-feature")
	mustGit(t, clone, "push", "-q", "origin", "main")

	mustGit(t, clone, "checkout", "-q", "-b", "open-feature")
	commit(t, clone, "open work")
	mustGit(t, clone, "push", "-q", "origin", "open-feature")

	mustGit(t, clone, "checkout", "-q", "main")
	mustGit(t, clone, "remote", "set-head", "origin", "-a")

	branches, err := readBranches(clone)
	if err != nil {
		t.Fatalf("readBranches: %v", err)
	}
	if len(branches) != 3 {
		t.Fatalf("got %d branches, want 3: %+v", len(branches), branches)
	}

	main, _ := branchByName(branches, "main")
	if !main.IsCurrent {
		t.Errorf("main.IsCurrent = false, want true")
	}
	if !main.IsDefault {
		t.Errorf("main.IsDefault = false, want true")
	}

	merged, _ := branchByName(branches, "merged-feature")
	if merged.IsCurrent {
		t.Errorf("merged-feature.IsCurrent = true, want false")
	}
	if !merged.MergedToDefault {
		t.Errorf("merged-feature.MergedToDefault = false, want true")
	}

	open, _ := branchByName(branches, "open-feature")
	if open.MergedToDefault {
		t.Errorf("open-feature.MergedToDefault = true, want false")
	}
	if open.IsDefault {
		t.Errorf("open-feature.IsDefault = true, want false")
	}
}

func TestReadBranchesNoOrigin(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q", "-b", "main", dir)
	mustGit(t, dir, "config", "user.email", "test@example.com")
	mustGit(t, dir, "config", "user.name", "Test")
	commit(t, dir, "initial")

	branches, err := readBranches(dir)
	if err != nil {
		t.Fatalf("readBranches: %v", err)
	}
	if len(branches) != 0 {
		t.Fatalf("got %d branches, want 0: %+v", len(branches), branches)
	}

	edges, err := readMergeEdges(dir, branches)
	if err != nil {
		t.Fatalf("readMergeEdges: %v", err)
	}
	if len(edges) != 0 {
		t.Fatalf("got %d merge edges, want 0: %+v", len(edges), edges)
	}
}

func TestReadMergeEdges(t *testing.T) {
	clone := newOriginFixture(t)
	commit(t, clone, "initial")
	mustGit(t, clone, "push", "-q", "origin", "HEAD:main")

	mustGit(t, clone, "checkout", "-q", "-b", "feature")
	commit(t, clone, "feature work")
	mustGit(t, clone, "push", "-q", "origin", "feature")

	mustGit(t, clone, "checkout", "-q", "main")
	mustGit(t, clone, "merge", "-q", "--no-ff", "-m", "merge feature", "feature")
	mustGit(t, clone, "push", "-q", "origin", "main")
	mergeSHA := mustGit(t, clone, "rev-parse", "HEAD")

	branches, err := readBranches(clone)
	if err != nil {
		t.Fatalf("readBranches: %v", err)
	}

	edges, err := readMergeEdges(clone, branches)
	if err != nil {
		t.Fatalf("readMergeEdges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("got %d merge edges, want 1: %+v", len(edges), edges)
	}
	got := edges[0]
	if got.Into != "main" || got.From != "feature" || got.Commit != mergeSHA {
		t.Errorf("edge = %+v, want Into=main From=feature Commit=%s", got, mergeSHA)
	}
}

func TestReadMergeEdgesDropsHistoryInheritedFromDefaultBranch(t *testing.T) {
	clone := newOriginFixture(t)
	commit(t, clone, "initial")
	mustGit(t, clone, "push", "-q", "origin", "HEAD:main")

	// A merge into main, before the feature branch below ever existed.
	mustGit(t, clone, "checkout", "-q", "-b", "earlier-feature")
	commit(t, clone, "earlier work")
	mustGit(t, clone, "checkout", "-q", "main")
	mustGit(t, clone, "merge", "-q", "--no-ff", "-m", "merge earlier-feature", "earlier-feature")
	mustGit(t, clone, "push", "-q", "origin", "main")

	// Forked from main after that merge, with no merges of its own: its
	// first-parent walk still passes through "merge earlier-feature", but
	// that merge is not this branch's own topology.
	mustGit(t, clone, "checkout", "-q", "-b", "later-feature")
	commit(t, clone, "later work")
	mustGit(t, clone, "push", "-q", "origin", "later-feature")

	mustGit(t, clone, "checkout", "-q", "main")
	mustGit(t, clone, "remote", "set-head", "origin", "-a")

	branches, err := readBranches(clone)
	if err != nil {
		t.Fatalf("readBranches: %v", err)
	}

	edges, err := readMergeEdges(clone, branches)
	if err != nil {
		t.Fatalf("readMergeEdges: %v", err)
	}
	for _, edge := range edges {
		if edge.Into == "later-feature" {
			t.Errorf("later-feature inherited a merge from main's history it wasn't party to: %+v", edge)
		}
	}

	main, _ := branchByName(branches, "main")
	var mainEdges int
	for _, edge := range edges {
		if edge.Into == main.Name {
			mainEdges++
		}
	}
	if mainEdges != 1 {
		t.Errorf("main has %d merge edges, want 1 (its own merge of earlier-feature)", mainEdges)
	}
}

func forkEdgeFor(edges []ForkEdge, branch string) (ForkEdge, bool) {
	for _, e := range edges {
		if e.Branch == branch {
			return e, true
		}
	}
	return ForkEdge{}, false
}

func TestInferForkEdgesSimpleFork(t *testing.T) {
	clone := newOriginFixture(t)
	commit(t, clone, "initial")
	mustGit(t, clone, "push", "-q", "origin", "HEAD:main")

	mustGit(t, clone, "checkout", "-q", "-b", "dev")
	commit(t, clone, "dev work")
	mustGit(t, clone, "push", "-q", "origin", "dev")

	mustGit(t, clone, "checkout", "-q", "main")
	mustGit(t, clone, "remote", "set-head", "origin", "-a")

	branches, err := readBranches(clone)
	if err != nil {
		t.Fatalf("readBranches: %v", err)
	}

	edges, err := inferForkEdges(clone, branches)
	if err != nil {
		t.Fatalf("inferForkEdges: %v", err)
	}

	dev, ok := forkEdgeFor(edges, "dev")
	if !ok {
		t.Fatal("no fork edge found for dev")
	}
	if dev.From != "main" {
		t.Errorf("dev.From = %q, want %q", dev.From, "main")
	}

	if _, ok := forkEdgeFor(edges, "main"); ok {
		t.Error("the default branch got a fork edge; it should never be assigned a parent")
	}
}

func TestInferForkEdgesPrefersNearestParentOverGrandparent(t *testing.T) {
	clone := newOriginFixture(t)
	commit(t, clone, "initial")
	mustGit(t, clone, "push", "-q", "origin", "HEAD:main")

	mustGit(t, clone, "checkout", "-q", "-b", "release")
	commit(t, clone, "release work")
	mustGit(t, clone, "push", "-q", "origin", "release")

	mustGit(t, clone, "checkout", "-q", "-b", "hotfix")
	commit(t, clone, "hotfix work")
	mustGit(t, clone, "push", "-q", "origin", "hotfix")

	mustGit(t, clone, "checkout", "-q", "main")
	mustGit(t, clone, "remote", "set-head", "origin", "-a")

	branches, err := readBranches(clone)
	if err != nil {
		t.Fatalf("readBranches: %v", err)
	}

	edges, err := inferForkEdges(clone, branches)
	if err != nil {
		t.Fatalf("inferForkEdges: %v", err)
	}

	release, ok := forkEdgeFor(edges, "release")
	if !ok || release.From != "main" {
		t.Errorf("release fork edge = %+v, ok=%v, want From=main", release, ok)
	}
	hotfix, ok := forkEdgeFor(edges, "hotfix")
	if !ok || hotfix.From != "release" {
		t.Errorf("hotfix fork edge = %+v, ok=%v, want From=release (nearest parent, not the grandparent main)", hotfix, ok)
	}
}

func TestInferForkEdgesUnrelatedHistoryYieldsNoEdge(t *testing.T) {
	clone := newOriginFixture(t)
	commit(t, clone, "initial")
	mustGit(t, clone, "push", "-q", "origin", "HEAD:main")

	mustGit(t, clone, "checkout", "-q", "--orphan", "unrelated")
	mustGit(t, clone, "commit", "--allow-empty", "-q", "-m", "unrelated history")
	mustGit(t, clone, "push", "-q", "origin", "unrelated")

	mustGit(t, clone, "checkout", "-q", "main")
	mustGit(t, clone, "remote", "set-head", "origin", "-a")

	branches, err := readBranches(clone)
	if err != nil {
		t.Fatalf("readBranches: %v", err)
	}

	edges, err := inferForkEdges(clone, branches)
	if err != nil {
		t.Fatalf("inferForkEdges: %v", err)
	}
	if _, ok := forkEdgeFor(edges, "unrelated"); ok {
		t.Error("a branch with no shared history got a fork edge; should have none, not a guess")
	}
}

func TestReadMergeEdgesDeletedSource(t *testing.T) {
	clone := newOriginFixture(t)
	commit(t, clone, "initial")
	mustGit(t, clone, "push", "-q", "origin", "HEAD:main")

	mustGit(t, clone, "checkout", "-q", "-b", "feature")
	commit(t, clone, "feature work")
	mustGit(t, clone, "push", "-q", "origin", "feature")

	mustGit(t, clone, "checkout", "-q", "main")
	mustGit(t, clone, "merge", "-q", "--no-ff", "-m", "merge feature", "feature")
	mustGit(t, clone, "push", "-q", "origin", "main")
	mergeSHA := mustGit(t, clone, "rev-parse", "HEAD")

	// Simulate the source branch having been deleted on the server and this
	// clone's remote-tracking cache already pruned, without needing a real
	// second remote round-trip.
	mustGit(t, clone, "update-ref", "-d", "refs/remotes/origin/feature")

	branches, err := readBranches(clone)
	if err != nil {
		t.Fatalf("readBranches: %v", err)
	}

	edges, err := readMergeEdges(clone, branches)
	if err != nil {
		t.Fatalf("readMergeEdges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("got %d merge edges, want 1: %+v", len(edges), edges)
	}
	got := edges[0]
	if got.Into != "main" || got.From != "" || got.Commit != mergeSHA {
		t.Errorf("edge = %+v, want Into=main From=\"\" Commit=%s", got, mergeSHA)
	}
}
