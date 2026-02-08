package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentstate "github.com/tinoosan/workbench-core/pkg/agent/state"
)

func TestResolveArtifactDisk_TeamMode(t *testing.T) {
	dataDir := "/tmp/wb"
	got := resolveArtifactDisk(dataDir, "team-1", "run-1", "/workspace/deliverables/2026-01-01/task-1/SUMMARY.md")
	want := filepath.Join(dataDir, "teams", "team-1", "workspace", "deliverables", "2026-01-01", "task-1", "SUMMARY.md")
	if got != want {
		t.Fatalf("expected team workspace path %q, got %q", want, got)
	}
}

func TestResolveArtifactDisk_RunMode(t *testing.T) {
	dataDir := "/tmp/wb"
	got := resolveArtifactDisk(dataDir, "", "run-1", "/workspace/out.txt")
	want := filepath.Join(dataDir, "agents", "run-1", "workspace", "out.txt")
	if got != want {
		t.Fatalf("expected run workspace path %q, got %q", want, got)
	}
}

func TestBuildArtifactTreeFromGroups_DayRoleKindTaskFiles(t *testing.T) {
	m := &monitorModel{artifactWorkspaceExpand: map[string]bool{}}
	groups := []agentstate.ArtifactGroup{
		{
			DayBucket: "2026-02-08",
			Role:      "ceo",
			TaskKind:  agentstate.TaskKindCallback,
			TaskID:    "callback-task-ceo-1",
			Goal:      "Review partner notes",
			Status:    "succeeded",
			Files: []agentstate.ArtifactRecord{
				{DisplayName: "SUMMARY.md", VPath: "/workspace/deliverables/2026-02-08/callback-task-ceo-1/SUMMARY.md", IsSummary: true},
				{DisplayName: "report.md", VPath: "/workspace/deliverables/2026-02-08/callback-task-ceo-1/report.md"},
			},
		},
	}

	tree := m.buildArtifactTreeFromGroups(groups)
	if len(tree) < 6 {
		t.Fatalf("expected day+role+kind+task+files nodes, got %d", len(tree))
	}
	if !tree[0].isDayHeader || tree[0].name != "2026-02-08" {
		t.Fatalf("expected day header first, got %+v", tree[0])
	}
	if !tree[1].isRoleHeader || tree[1].name != "ceo" {
		t.Fatalf("expected role header second, got %+v", tree[1])
	}
	if !tree[2].isKindHeader || tree[2].name != "Callback Tasks" {
		t.Fatalf("expected callback kind header, got %+v", tree[2])
	}
	if !tree[3].isTaskHeader || tree[3].taskID != "callback-task-ceo-1" {
		t.Fatalf("expected task header, got %+v", tree[3])
	}
	if tree[4].name != "SUMMARY.md" || !tree[4].isSummary {
		t.Fatalf("expected summary file leaf, got %+v", tree[4])
	}
}

func TestRebuildTree_PreservesSelectionOnExpandCollapse(t *testing.T) {
	m := &monitorModel{artifactWorkspaceExpand: map[string]bool{}}
	groups := []agentstate.ArtifactGroup{
		{
			DayBucket: "2026-02-08",
			Role:      "ceo",
			TaskKind:  agentstate.TaskKindTask,
			TaskID:    "task-1",
			Goal:      "Analyze",
			Status:    "succeeded",
			Files: []agentstate.ArtifactRecord{
				{DisplayName: "SUMMARY.md", VPath: "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", IsSummary: true},
			},
		},
	}
	m.artifactAllTree = m.buildArtifactTreeFromGroups(groups)
	m.applyArtifactVisibilityAndSearch()

	idx := m.findArtifactNodeByKey("role:2026-02-08:ceo")
	if idx < 0 {
		t.Fatalf("expected role node")
	}
	m.artifactSelected = idx
	m.artifactWorkspaceExpand["role:2026-02-08:ceo"] = true
	m.rebuildArtifactTreeWithAnchor("role:2026-02-08:ceo", idx)
	if m.artifactTree[m.artifactSelected].key != "role:2026-02-08:ceo" {
		t.Fatalf("selection moved unexpectedly to %q", m.artifactTree[m.artifactSelected].key)
	}

	m.artifactWorkspaceExpand["role:2026-02-08:ceo"] = false
	m.rebuildArtifactTreeWithAnchor("role:2026-02-08:ceo", m.artifactSelected)
	if m.artifactTree[m.artifactSelected].key != "role:2026-02-08:ceo" {
		t.Fatalf("selection should stay on collapsed node, got %q", m.artifactTree[m.artifactSelected].key)
	}
}

func TestSearchFilter_FindsInCollapsedBranches(t *testing.T) {
	m := &monitorModel{artifactWorkspaceExpand: map[string]bool{}}
	groups := []agentstate.ArtifactGroup{
		{
			DayBucket: "2026-02-08",
			Role:      "ceo",
			TaskKind:  agentstate.TaskKindTask,
			TaskID:    "task-1",
			Goal:      "Analyze",
			Status:    "succeeded",
			ProducedAt: time.Now(),
			Files: []agentstate.ArtifactRecord{
				{DisplayName: "SUMMARY.md", VPath: "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", IsSummary: true},
				{DisplayName: "report.md", VPath: "/workspace/deliverables/2026-02-08/task-1/report.md"},
			},
		},
	}
	m.artifactAllTree = m.buildArtifactTreeFromGroups(groups)
	// Collapse everything.
	for _, n := range m.artifactAllTree {
		if n.isHeader {
			m.artifactWorkspaceExpand[n.key] = false
		}
	}
	m.artifactSearchQuery = "summary"
	m.applyArtifactVisibilityAndSearch()
	if len(m.artifactTree) == 0 {
		t.Fatalf("expected non-empty filtered tree")
	}
	found := false
	for _, n := range m.artifactTree {
		if strings.HasPrefix(n.key, "file:") && strings.EqualFold(n.name, "SUMMARY.md") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to find summary in collapsed branches, got %+v", m.artifactTree)
	}
}
