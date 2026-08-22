package main

import "testing"

// initRepo creates a fresh repository at dir with a deterministic branch name
// and local commit identity, so tests don't depend on the machine's git config.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	mustGit(t, dir, "config", "user.name", "Test")
}

// commit makes an empty commit so fixtures don't need real file content.
func commit(t *testing.T, dir, message string) string {
	t.Helper()
	mustGit(t, dir, "commit", "--allow-empty", "-m", message)
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
	dir := t.TempDir()
	initRepo(t, dir)
	commit(t, dir, "initial")

	mustGit(t, dir, "checkout", "-b", "merged-feature")
	commit(t, dir, "feature work")
	mustGit(t, dir, "checkout", "main")
	mustGit(t, dir, "merge", "--no-ff", "-m", "merge feature", "merged-feature")

	mustGit(t, dir, "checkout", "-b", "open-feature")
	commit(t, dir, "open work")
	mustGit(t, dir, "checkout", "main")

	branches, err := readBranches(dir)
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

	merged, _ := branchByName(branches, "merged-feature")
	if merged.IsCurrent {
		t.Errorf("merged-feature.IsCurrent = true, want false")
	}
	if !merged.MergedToHead {
		t.Errorf("merged-feature.MergedToHead = false, want true")
	}

	open, _ := branchByName(branches, "open-feature")
	if open.MergedToHead {
		t.Errorf("open-feature.MergedToHead = true, want false")
	}
}

func TestReadBranchesUnbornHead(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)

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
	dir := t.TempDir()
	initRepo(t, dir)
	commit(t, dir, "initial")

	mustGit(t, dir, "checkout", "-b", "feature")
	commit(t, dir, "feature work")
	mustGit(t, dir, "checkout", "main")
	mustGit(t, dir, "merge", "--no-ff", "-m", "merge feature", "feature")
	mergeSHA := mustGit(t, dir, "rev-parse", "HEAD")

	branches, err := readBranches(dir)
	if err != nil {
		t.Fatalf("readBranches: %v", err)
	}

	edges, err := readMergeEdges(dir, branches)
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
	dir := t.TempDir()
	initRepo(t, dir)
	commit(t, dir, "initial")

	mustGit(t, dir, "checkout", "-b", "feature")
	commit(t, dir, "feature work")
	mustGit(t, dir, "checkout", "main")
	mustGit(t, dir, "merge", "--no-ff", "-m", "merge feature", "feature")
	mergeSHA := mustGit(t, dir, "rev-parse", "HEAD")
	mustGit(t, dir, "branch", "-D", "feature")

	branches, err := readBranches(dir)
	if err != nil {
		t.Fatalf("readBranches: %v", err)
	}

	edges, err := readMergeEdges(dir, branches)
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
