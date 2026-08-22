package main

import (
	"reflect"
	"testing"
	"time"
)

func TestReconcileLaneOrderKeepsSavedPositions(t *testing.T) {
	branches := []Branch{
		{Name: "main", IsDefault: true},
		{Name: "feature-a"},
		{Name: "feature-b"},
	}
	got := reconcileLaneOrder([]string{"feature-b", "main"}, branches)
	want := []string{"feature-b", "main", "feature-a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reconcileLaneOrder = %v, want %v", got, want)
	}
}

func TestReconcileLaneOrderDropsMissingNames(t *testing.T) {
	branches := []Branch{{Name: "main", IsDefault: true}}
	got := reconcileLaneOrder([]string{"deleted-branch", "main"}, branches)
	want := []string{"main"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reconcileLaneOrder = %v, want %v", got, want)
	}
}

func TestReconcileLaneOrderAppendsNewBranchesDefaultFirst(t *testing.T) {
	now := time.Now()
	branches := []Branch{
		{Name: "old-feature", CommitterDate: now.Add(-48 * time.Hour)},
		{Name: "main", IsDefault: true, CommitterDate: now.Add(-1 * time.Hour)},
		{Name: "new-feature", CommitterDate: now},
	}
	// Nothing saved yet: every branch is "new" and gets the default ordering.
	got := reconcileLaneOrder(nil, branches)
	want := []string{"main", "new-feature", "old-feature"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reconcileLaneOrder = %v, want %v", got, want)
	}
}

func TestReconcileLaneOrderAppendsUnorderedBranchesAfterSaved(t *testing.T) {
	now := time.Now()
	branches := []Branch{
		{Name: "main", IsDefault: true, CommitterDate: now},
		{Name: "feature-a", CommitterDate: now},
		{Name: "feature-b", CommitterDate: now.Add(time.Hour)}, // added after the order was saved
	}
	// The user previously arranged feature-a above main; feature-b appeared
	// since, so it lands after both, not disturbing the saved arrangement.
	got := reconcileLaneOrder([]string{"feature-a", "main"}, branches)
	want := []string{"feature-a", "main", "feature-b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reconcileLaneOrder = %v, want %v", got, want)
	}
}

func TestReconcileLaneOrderNoBranchesReturnsNil(t *testing.T) {
	if got := reconcileLaneOrder([]string{"main"}, nil); got != nil {
		t.Errorf("reconcileLaneOrder with no branches = %v, want nil", got)
	}
}

func TestSetLaneOrderPersists(t *testing.T) {
	s := newTestStore(t)
	repo, _, err := s.add(makeRepo(t, "project"))
	if err != nil {
		t.Fatal(err)
	}

	order := []string{"feature-b", "main", "feature-a"}
	if err := s.setLaneOrder(repo.ID, order); err != nil {
		t.Fatal(err)
	}

	reloaded := &store{path: s.path}
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	got := reloaded.snapshot().Repos[0].LaneOrder
	if !reflect.DeepEqual(got, order) {
		t.Errorf("reloaded lane order = %v, want %v", got, order)
	}
}

func TestSetLaneOrderUnknownIDFails(t *testing.T) {
	s := newTestStore(t)
	if err := s.setLaneOrder("nope", []string{"main"}); err == nil {
		t.Error("expected an error when setting lane order for an id that is not present")
	}
}
