package team

import (
	"context"
	"testing"
)

func TestStateManager_QueueAndMarkModelChange(t *testing.T) {
	var saved Manifest
	store := &mockManifestStore{
		save: func(ctx context.Context, m Manifest) error {
			saved = m
			return nil
		},
	}
	initial := Manifest{TeamID: "team-1", TeamModel: "gpt-4"}
	mgr := NewStateManager(store, initial)
	ctx := context.Background()

	snap := mgr.ManifestSnapshot()
	if snap.TeamID != "team-1" || snap.TeamModel != "gpt-4" {
		t.Fatalf("snapshot = %+v", snap)
	}

	err := mgr.QueueModelChange(ctx, "gpt-5", "rpc")
	if err != nil {
		t.Fatalf("QueueModelChange: %v", err)
	}
	snap = mgr.ManifestSnapshot()
	if snap.ModelChange == nil || snap.ModelChange.Status != "pending" || snap.ModelChange.RequestedModel != "gpt-5" {
		t.Fatalf("after queue: %+v", snap.ModelChange)
	}
	if saved.ModelChange == nil {
		t.Fatal("store.Save was not called with updated manifest")
	}

	err = mgr.MarkModelApplied(ctx, "gpt-5")
	if err != nil {
		t.Fatalf("MarkModelApplied: %v", err)
	}
	snap = mgr.ManifestSnapshot()
	if snap.TeamModel != "gpt-5" || snap.ModelChange == nil || snap.ModelChange.Status != "applied" {
		t.Fatalf("after mark applied: TeamModel=%q ModelChange=%+v", snap.TeamModel, snap.ModelChange)
	}

	err = mgr.MarkModelFailed(ctx, "gpt-6", context.DeadlineExceeded)
	if err != nil {
		t.Fatalf("MarkModelFailed: %v", err)
	}
	snap = mgr.ManifestSnapshot()
	if snap.ModelChange == nil || snap.ModelChange.Status != "failed" || snap.ModelChange.Error == "" {
		t.Fatalf("after mark failed: %+v", snap.ModelChange)
	}
}
