package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// storeVersion is the schema version of the on-disk workspace file. Bump it when
// the shape of Workspace changes in a way that needs migrating on load.
const storeVersion = 1

// Repo is a single Git repository the user has added to the workspace.
type Repo struct {
	// ID is derived from Path and is stable across restarts, so the frontend can
	// use it as a key and to address a repo in service calls.
	ID string `json:"id"`
	// Name is the display label — the repository folder's name.
	Name string `json:"name"`
	// Path is the absolute path to the root of the working tree (or to the
	// repository itself, if bare).
	Path    string    `json:"path"`
	AddedAt time.Time `json:"addedAt"`
	// Missing reports that Path could not be found on disk. It is recomputed on
	// every read, so the value persisted alongside the rest is never trusted: an
	// unplugged drive or an unmounted share should grey a repo out, not silently
	// drop it from the workspace.
	Missing bool `json:"missing"`
}

// Workspace is the whole persisted state: the repositories the user has added,
// and which one is currently selected.
type Workspace struct {
	Version  int    `json:"version"`
	Repos    []Repo `json:"repos"`
	ActiveID string `json:"activeId"`
}

// store holds the workspace and keeps it mirrored to a JSON file. Service methods
// are called from arbitrary goroutines, so every exported operation takes the lock.
type store struct {
	mu   sync.Mutex
	path string
	data Workspace
}

// newStore locates the workspace file under the user's config directory. It does
// not touch the disk.
func newStore() (*store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locating user config directory: %w", err)
	}
	return &store{
		path: filepath.Join(dir, "kombu", "workspace.json"),
		data: Workspace{Version: storeVersion},
	}, nil
}

// load reads the workspace file. A missing file is not an error — it just means
// this is a first run. A corrupt file is moved aside rather than reported, so a
// bad write can never leave the app unable to start.
func (s *store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	blob, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", s.path, err)
	}

	var data Workspace
	if err := json.Unmarshal(blob, &data); err != nil {
		quarantine := s.path + ".corrupt"
		if renameErr := os.Rename(s.path, quarantine); renameErr != nil {
			return fmt.Errorf("workspace file %s is corrupt and could not be moved aside: %w", s.path, renameErr)
		}
		return fmt.Errorf("workspace file was corrupt and has been moved to %s; starting with an empty workspace", quarantine)
	}

	data.Version = storeVersion
	s.data = data
	s.pruneActiveLocked()
	return nil
}

// snapshot returns a copy of the workspace with liveness re-checked, safe to hand
// to the frontend.
func (s *store) snapshot() Workspace {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

func (s *store) snapshotLocked() Workspace {
	repos := make([]Repo, len(s.data.Repos))
	copy(repos, s.data.Repos)
	for i := range repos {
		repos[i].Missing = !dirExists(repos[i].Path)
	}
	return Workspace{
		Version:  storeVersion,
		Repos:    repos,
		ActiveID: s.data.ActiveID,
	}
}

// add resolves dir to the root of its repository and appends it. An entry that is
// already present is selected rather than duplicated; added reports which
// happened.
func (s *store) add(dir string) (repo Repo, added bool, err error) {
	root, err := repoRoot(dir)
	if err != nil {
		return Repo{}, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := repoID(root)
	for i := range s.data.Repos {
		if s.data.Repos[i].ID == id {
			s.data.ActiveID = id
			existing := s.data.Repos[i]
			existing.Missing = !dirExists(existing.Path)
			return existing, false, s.persistLocked()
		}
	}

	repo = Repo{
		ID:      id,
		Name:    repoDisplayName(root),
		Path:    root,
		AddedAt: time.Now().UTC(),
	}
	s.data.Repos = append(s.data.Repos, repo)
	s.data.ActiveID = id
	return repo, true, s.persistLocked()
}

// remove drops a repo. Removing the active tab selects a neighbour, preferring the
// one to its right, which is how tab strips are expected to behave.
func (s *store) remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := -1
	for i := range s.data.Repos {
		if s.data.Repos[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("no repository with id %q", id)
	}

	s.data.Repos = append(s.data.Repos[:index], s.data.Repos[index+1:]...)

	if s.data.ActiveID == id {
		s.data.ActiveID = ""
		if len(s.data.Repos) > 0 {
			next := index
			if next > len(s.data.Repos)-1 {
				next = len(s.data.Repos) - 1
			}
			s.data.ActiveID = s.data.Repos[next].ID
		}
	}
	return s.persistLocked()
}

// repo looks up a single repository by id.
func (s *store) repo(id string) (Repo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.data.Repos {
		if s.data.Repos[i].ID == id {
			return s.data.Repos[i], nil
		}
	}
	return Repo{}, fmt.Errorf("no repository with id %q", id)
}

func (s *store) setActive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.data.Repos {
		if s.data.Repos[i].ID == id {
			if s.data.ActiveID == id {
				return nil
			}
			s.data.ActiveID = id
			return s.persistLocked()
		}
	}
	return fmt.Errorf("no repository with id %q", id)
}

// pruneActiveLocked keeps ActiveID pointing at a repo that actually exists.
func (s *store) pruneActiveLocked() {
	for i := range s.data.Repos {
		if s.data.Repos[i].ID == s.data.ActiveID {
			return
		}
	}
	s.data.ActiveID = ""
	if len(s.data.Repos) > 0 {
		s.data.ActiveID = s.data.Repos[0].ID
	}
}

// persistLocked writes the workspace out via a temporary file so that an
// interrupted write cannot leave a half-written file behind. os.Rename replaces
// the destination on every platform Wails targets.
func (s *store) persistLocked() error {
	s.data.Version = storeVersion

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	blob, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding workspace: %w", err)
	}
	blob = append(blob, '\n')

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o644); err != nil {
		return fmt.Errorf("writing workspace: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replacing workspace file: %w", err)
	}
	return nil
}

// repoRoot walks up from dir to find the root of a Git working tree or a bare
// repository, mirroring how the git CLI discovers a repository from any
// subdirectory: picking `myrepo/src/lib` should add `myrepo`.
func repoRoot(dir string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", dir, err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("%s cannot be read", abs)
	}
	if !info.IsDir() {
		abs = filepath.Dir(abs)
	}

	for current := abs; ; {
		// `.git` is a directory in a normal clone and a file in a worktree or
		// submodule, so its kind is deliberately not checked.
		if pathExists(filepath.Join(current, ".git")) {
			return current, nil
		}
		if isBareRepo(current) {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("%s is not a Git repository (no .git found here or in any parent directory)", abs)
		}
		current = parent
	}
}

// isBareRepo recognises a bare repository — a mirror clone has no working tree
// and so no .git, but still carries these three entries.
func isBareRepo(dir string) bool {
	return pathExists(filepath.Join(dir, "HEAD")) &&
		dirExists(filepath.Join(dir, "objects")) &&
		dirExists(filepath.Join(dir, "refs"))
}

// repoDisplayName is the folder name, with the .git suffix of a bare clone dropped.
func repoDisplayName(root string) string {
	name := filepath.Base(root)
	if trimmed := strings.TrimSuffix(name, ".git"); trimmed != "" {
		name = trimmed
	}
	// A drive or filesystem root has no usable base name; show the path instead.
	if name == "" || name == "." || name == string(filepath.Separator) {
		return root
	}
	return name
}

// repoID is a short stable hash of the repository path.
func repoID(root string) string {
	sum := sha256.Sum256([]byte(pathKey(root)))
	return hex.EncodeToString(sum[:])[:12]
}

// pathKey normalises a path for identity comparisons. Windows and macOS default to
// case-insensitive filesystems, where C:\Repo and c:\repo are the same directory
// and must not become two tabs.
func pathKey(path string) string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(path)
	}
	return path
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
