package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// newTestStore builds a store backed by a file under t.TempDir, bypassing the
// user config directory.
func newTestStore(t *testing.T) *store {
	t.Helper()
	return &store{
		path: filepath.Join(t.TempDir(), "workspace.json"),
		data: Workspace{Version: storeVersion},
	}
}

// makeRepo creates a directory containing a .git directory and returns its path.
func makeRepo(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRepoRootFromSubdirectory(t *testing.T) {
	root := makeRepo(t, "project")
	nested := filepath.Join(root, "src", "internal")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := repoRoot(nested)
	if err != nil {
		t.Fatalf("repoRoot(%s) returned error: %v", nested, err)
	}
	if got != root {
		t.Errorf("repoRoot(%s) = %s, want %s", nested, got, root)
	}
}

func TestRepoRootAcceptsGitFile(t *testing.T) {
	// A linked worktree or submodule has a .git *file*, not a directory.
	root := filepath.Join(t.TempDir(), "worktree")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: ../.git/worktrees/wt\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := repoRoot(root)
	if err != nil {
		t.Fatalf("repoRoot returned error for a .git file: %v", err)
	}
	if got != root {
		t.Errorf("repoRoot = %s, want %s", got, root)
	}
}

func TestRepoRootAcceptsBareRepo(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mirror.git")
	for _, dir := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := repoRoot(root)
	if err != nil {
		t.Fatalf("repoRoot returned error for a bare repo: %v", err)
	}
	if got != root {
		t.Errorf("repoRoot = %s, want %s", got, root)
	}
	if name := repoDisplayName(root); name != "mirror" {
		t.Errorf("repoDisplayName = %q, want %q (the .git suffix should be dropped)", name, "mirror")
	}
}

func TestRepoRootRejectsPlainDirectory(t *testing.T) {
	dir := t.TempDir()
	if _, err := repoRoot(dir); err == nil {
		t.Fatal("expected an error for a directory that is not a Git repository")
	}
}

func TestAddIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	root := makeRepo(t, "project")

	first, added, err := s.add(root)
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Error("first add reported added=false")
	}

	second, added, err := s.add(root)
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Error("re-adding the same repository reported added=true")
	}
	if second.ID != first.ID {
		t.Errorf("ids differ across adds: %s vs %s", first.ID, second.ID)
	}
	if got := len(s.snapshot().Repos); got != 1 {
		t.Errorf("workspace holds %d repos, want 1", got)
	}
}

func TestAddDedupesSubdirectoryOfSameRepo(t *testing.T) {
	s := newTestStore(t)
	root := makeRepo(t, "project")
	nested := filepath.Join(root, "pkg")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.add(root); err != nil {
		t.Fatal(err)
	}
	if _, added, err := s.add(nested); err != nil {
		t.Fatal(err)
	} else if added {
		t.Error("adding a subdirectory of an open repository created a second entry")
	}
}

func TestAddDedupesCaseInsensitivelyOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("path comparison is case-sensitive on this platform")
	}
	s := newTestStore(t)
	root := makeRepo(t, "Project")

	if _, _, err := s.add(root); err != nil {
		t.Fatal(err)
	}
	if _, added, err := s.add(strings.ToLower(root)); err != nil {
		t.Fatal(err)
	} else if added {
		t.Error("the same repository added with different casing created a second entry")
	}
}

func TestAddSelectsWhatItAdds(t *testing.T) {
	s := newTestStore(t)
	a := makeRepo(t, "a")
	b := makeRepo(t, "b")

	repoA, _, err := s.add(a)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.snapshot().ActiveID; got != repoA.ID {
		t.Errorf("active id = %q, want the repo just added (%q)", got, repoA.ID)
	}

	repoB, _, err := s.add(b)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.snapshot().ActiveID; got != repoB.ID {
		t.Errorf("active id = %q, want the repo just added (%q)", got, repoB.ID)
	}
}

func TestRemoveActiveSelectsRightNeighbour(t *testing.T) {
	s := newTestStore(t)
	var ids []string
	for _, name := range []string{"a", "b", "c"} {
		repo, _, err := s.add(makeRepo(t, name))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, repo.ID)
	}

	// Select the middle tab, then close it: the one to its right takes over.
	if err := s.setActive(ids[1]); err != nil {
		t.Fatal(err)
	}
	if err := s.remove(ids[1]); err != nil {
		t.Fatal(err)
	}
	if got := s.snapshot().ActiveID; got != ids[2] {
		t.Errorf("active id = %q, want the right-hand neighbour %q", got, ids[2])
	}

	// Closing the last tab falls back to the one on its left.
	if err := s.remove(ids[2]); err != nil {
		t.Fatal(err)
	}
	if got := s.snapshot().ActiveID; got != ids[0] {
		t.Errorf("active id = %q, want %q", got, ids[0])
	}

	// Closing the only remaining tab leaves nothing selected.
	if err := s.remove(ids[0]); err != nil {
		t.Fatal(err)
	}
	snapshot := s.snapshot()
	if len(snapshot.Repos) != 0 || snapshot.ActiveID != "" {
		t.Errorf("workspace not empty after removing every repo: %+v", snapshot)
	}
}

func TestRemoveUnknownIDFails(t *testing.T) {
	s := newTestStore(t)
	if err := s.remove("nope"); err == nil {
		t.Error("expected an error when removing an id that is not present")
	}
}

func TestPersistAndReload(t *testing.T) {
	s := newTestStore(t)
	root := makeRepo(t, "project")
	repo, _, err := s.add(root)
	if err != nil {
		t.Fatal(err)
	}

	// A second write must replace the file, not fail because it already exists —
	// os.Rename semantics differ per platform.
	if _, _, err := s.add(makeRepo(t, "other")); err != nil {
		t.Fatalf("second persist failed: %v", err)
	}

	reloaded := &store{path: s.path}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	snapshot := reloaded.snapshot()
	if len(snapshot.Repos) != 2 {
		t.Fatalf("reloaded %d repos, want 2", len(snapshot.Repos))
	}
	if snapshot.Repos[0].ID != repo.ID || snapshot.Repos[0].Path != root {
		t.Errorf("reloaded first repo = %+v, want id %s at %s", snapshot.Repos[0], repo.ID, root)
	}
	if snapshot.Version != storeVersion {
		t.Errorf("reloaded version = %d, want %d", snapshot.Version, storeVersion)
	}
	if _, err := os.Stat(s.path + ".tmp"); err == nil {
		t.Error("temporary file was left behind after persisting")
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	s := newTestStore(t)
	if err := s.load(); err != nil {
		t.Errorf("load of a non-existent workspace returned %v, want nil", err)
	}
	if got := len(s.snapshot().Repos); got != 0 {
		t.Errorf("expected an empty workspace, got %d repos", got)
	}
}

func TestLoadQuarantinesCorruptFile(t *testing.T) {
	s := newTestStore(t)
	if err := os.WriteFile(s.path, []byte("{ this is not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The error is informational — what matters is that the store is usable.
	if err := s.load(); err == nil {
		t.Error("expected load to report that the file was corrupt")
	}
	if _, err := os.Stat(s.path + ".corrupt"); err != nil {
		t.Errorf("corrupt file was not moved aside: %v", err)
	}
	if _, _, err := s.add(makeRepo(t, "project")); err != nil {
		t.Errorf("store unusable after a corrupt load: %v", err)
	}
}

func TestSnapshotFlagsMissingPaths(t *testing.T) {
	s := newTestStore(t)
	root := makeRepo(t, "project")
	if _, _, err := s.add(root); err != nil {
		t.Fatal(err)
	}

	if s.snapshot().Repos[0].Missing {
		t.Error("a repository that exists was reported as missing")
	}

	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	snapshot := s.snapshot()
	if !snapshot.Repos[0].Missing {
		t.Error("a repository whose folder was deleted was not reported as missing")
	}
	if len(snapshot.Repos) != 1 {
		t.Error("a missing repository was dropped from the workspace instead of flagged")
	}
}

func TestPersistedFileIsReadableJSON(t *testing.T) {
	s := newTestStore(t)
	if _, _, err := s.add(makeRepo(t, "project")); err != nil {
		t.Fatal(err)
	}

	blob, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal(blob, &generic); err != nil {
		t.Fatalf("workspace file is not valid JSON: %v", err)
	}
	for _, key := range []string{"version", "repos", "activeId"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("workspace file is missing the %q key", key)
		}
	}
}

func TestDialogCancelled(t *testing.T) {
	cases := []struct {
		name string
		path string
		err  error
		want bool
	}{
		{name: "windows cancel", err: errString("cancelled by user"), want: true},
		{name: "empty selection", path: "", want: true},
		{name: "cleaned empty selection", path: ".", want: true},
		{name: "real failure", err: errString("access is denied"), want: false},
		{name: "a chosen folder", path: `C:\src\project`, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dialogCancelled(tc.path, tc.err); got != tc.want {
				t.Errorf("dialogCancelled(%q, %v) = %v, want %v", tc.path, tc.err, got, tc.want)
			}
		})
	}
}

type errString string

func (e errString) Error() string { return string(e) }
