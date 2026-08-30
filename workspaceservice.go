package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// WorkspaceService owns the set of repositories the user is working with and the
// JSON file they are persisted to. It is the frontend's only route to that state.
type WorkspaceService struct {
	store *store
}

func NewWorkspaceService() *WorkspaceService {
	return &WorkspaceService{}
}

func (s *WorkspaceService) ServiceName() string {
	return "WorkspaceService"
}

// ServiceStartup loads the persisted workspace. A failure to read it is logged
// rather than returned: refusing to start the app because of an unreadable
// preferences file would be a worse outcome than starting empty.
func (s *WorkspaceService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	st, err := newStore()
	if err != nil {
		return err
	}
	if err := st.load(); err != nil {
		if logger := application.Get().Logger; logger != nil {
			logger.Warn("could not load saved workspace", "error", err)
		}
	}
	s.store = st
	return nil
}

// AddRepositoryResult is what the frontend gets back from a single click of the
// plus button. It always carries the resulting Workspace so the UI can replace its
// state from one call, plus enough detail to explain what happened.
type AddRepositoryResult struct {
	Workspace Workspace `json:"workspace"`
	// Repo is the repository that was added or, when Duplicate is set, the
	// existing entry that is now selected. It is nil if the user cancelled.
	Repo *Repo `json:"repo"`
	// Cancelled reports that the user dismissed the folder picker.
	Cancelled bool `json:"cancelled"`
	// Duplicate reports that the chosen folder was already in the workspace.
	Duplicate bool `json:"duplicate"`
}

// GetWorkspace returns the current repositories and selection. Called once when
// the UI mounts.
func (s *WorkspaceService) GetWorkspace() (Workspace, error) {
	if s.store == nil {
		return Workspace{}, fmt.Errorf("workspace service is not initialised")
	}
	return s.store.snapshot(), nil
}

// AddRepository opens a native folder picker and adds the chosen directory. If the
// user picks a subdirectory of a repository, the repository root is added.
func (s *WorkspaceService) AddRepository() (AddRepositoryResult, error) {
	if s.store == nil {
		return AddRepositoryResult{}, fmt.Errorf("workspace service is not initialised")
	}

	dir, err := application.Get().Dialog.OpenFile().
		SetTitle("Select a Git repository").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		CanCreateDirectories(false).
		PromptForSingleSelection()

	if dialogCancelled(dir, err) {
		return AddRepositoryResult{Workspace: s.store.snapshot(), Cancelled: true}, nil
	}
	if err != nil {
		return AddRepositoryResult{}, fmt.Errorf("opening folder picker: %w", err)
	}

	repo, added, err := s.store.add(dir)
	if err != nil {
		return AddRepositoryResult{}, err
	}

	return AddRepositoryResult{
		Workspace: s.store.snapshot(),
		Repo:      &repo,
		Duplicate: !added,
	}, nil
}

// RemoveRepository forgets a repository. Nothing on disk is touched — only this
// app's list of what to show.
func (s *WorkspaceService) RemoveRepository(id string) (Workspace, error) {
	if s.store == nil {
		return Workspace{}, fmt.Errorf("workspace service is not initialised")
	}
	if err := s.store.remove(id); err != nil {
		return Workspace{}, err
	}
	return s.store.snapshot(), nil
}

// SetActiveRepository records which tab is selected, so the same one is open on
// the next launch.
func (s *WorkspaceService) SetActiveRepository(id string) (Workspace, error) {
	if s.store == nil {
		return Workspace{}, fmt.Errorf("workspace service is not initialised")
	}
	if err := s.store.setActive(id); err != nil {
		return Workspace{}, err
	}
	return s.store.snapshot(), nil
}

// BranchInfo is a repository's origin branches and the merge edges between
// them, read live from refs/remotes/origin/* — never persisted, never fetched.
type BranchInfo struct {
	Branches []Branch    `json:"branches"`
	Merges   []MergeEdge `json:"merges"`
	// Forks is a best-guess "cut from" edge per non-default branch — see
	// inferForkEdges. A branch missing from this list simply has no
	// confident candidate parent, not an error.
	Forks []ForkEdge `json:"forks"`
	// LaneOrder is the branch names in the order the lane view should render
	// them: the repo's saved LaneOrder reconciled against the branches just
	// read, so it always names every branch above exactly once.
	LaneOrder []string `json:"laneOrder"`
}

// GetBranches reads repo id's origin branches and merge topology from whatever
// refs/remotes/origin/* already holds locally.
func (s *WorkspaceService) GetBranches(id string) (BranchInfo, error) {
	if s.store == nil {
		return BranchInfo{}, fmt.Errorf("workspace service is not initialised")
	}
	repo, err := s.store.repo(id)
	if err != nil {
		return BranchInfo{}, err
	}
	branches, err := readBranches(repo.Path)
	if err != nil {
		return BranchInfo{}, err
	}
	merges, err := readMergeEdges(repo.Path, branches)
	if err != nil {
		return BranchInfo{}, err
	}
	forks, err := inferForkEdges(repo.Path, branches, merges)
	if err != nil {
		return BranchInfo{}, err
	}
	return BranchInfo{
		Branches:  branches,
		Merges:    merges,
		Forks:     forks,
		LaneOrder: reconcileLaneOrder(repo.LaneOrder, branches),
	}, nil
}

// SetLaneOrder saves repo id's lane order — the branch names, top to bottom,
// the user dragged into place. Names not among the repo's current branches
// are harmless (GetBranches's reconciliation drops them on the next read),
// so no validation against live branches happens here.
func (s *WorkspaceService) SetLaneOrder(id string, order []string) (Workspace, error) {
	if s.store == nil {
		return Workspace{}, fmt.Errorf("workspace service is not initialised")
	}
	if err := s.store.setLaneOrder(id, order); err != nil {
		return Workspace{}, err
	}
	return s.store.snapshot(), nil
}

// dialogCancelled distinguishes "the user closed the picker" from a real failure.
// The two cases are not reported consistently across platforms: Windows returns a
// "cancelled by user" error from an internal package that cannot be compared
// against directly, while the others return an empty selection.
func dialogCancelled(path string, err error) bool {
	if err != nil {
		return strings.Contains(strings.ToLower(err.Error()), "cancel")
	}
	trimmed := strings.TrimSpace(path)
	return trimmed == "" || trimmed == "."
}
