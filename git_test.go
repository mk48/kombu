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
