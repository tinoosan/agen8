package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDiskUserProfileStore_Basics(t *testing.T) {
	tmp := t.TempDir()
	s, err := NewDiskUserProfileStoreFromDir(filepath.Join(tmp, "user_profile"))
	if err != nil {
		t.Fatalf("NewDiskUserProfileStoreFromDir: %v", err)
	}
	ctx := context.Background()
	if err := s.AppendUserProfile(ctx, "hello"); err != nil {
		t.Fatalf("AppendUserProfile: %v", err)
	}
	got, err := s.GetUserProfile(ctx)
	if err != nil {
		t.Fatalf("GetUserProfile: %v", err)
	}
	if got == "" {
		t.Fatalf("expected non-empty user profile")
	}
}

