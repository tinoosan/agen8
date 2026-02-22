package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinoosan/agen8/pkg/fsutil"
)

func TestDiskWorkspacePreparer_PrepareTeamWorkspace(t *testing.T) {
	dataDir := t.TempDir()
	p := NewDiskWorkspacePreparer(dataDir)
	teamID := "team-1"

	err := p.PrepareTeamWorkspace(context.Background(), teamID)
	if err != nil {
		t.Fatalf("PrepareTeamWorkspace: %v", err)
	}

	dir := fsutil.GetTeamWorkspaceDir(dataDir, teamID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("workspace dir was not created: %s", dir)
	}
}

func TestDiskWorkspacePreparer_PrepareTeamWorkspace_Idempotent(t *testing.T) {
	dataDir := t.TempDir()
	p := NewDiskWorkspacePreparer(dataDir)
	teamID := "team-2"

	// First call creates
	if err := p.PrepareTeamWorkspace(context.Background(), teamID); err != nil {
		t.Fatalf("first PrepareTeamWorkspace: %v", err)
	}
	// Create a file to verify second call doesn't wipe
	wsDir := fsutil.GetTeamWorkspaceDir(dataDir, teamID)
	marker := filepath.Join(wsDir, "marker.txt")
	if err := os.WriteFile(marker, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	// Second call should be idempotent
	if err := p.PrepareTeamWorkspace(context.Background(), teamID); err != nil {
		t.Fatalf("second PrepareTeamWorkspace: %v", err)
	}
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		t.Fatal("marker file was removed by second PrepareTeamWorkspace")
	}
}
