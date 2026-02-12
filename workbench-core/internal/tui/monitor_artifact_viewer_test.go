package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/pkg/protocol"
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

func TestBuildArtifactTreeFromGroups_FlatRoleFiles(t *testing.T) {
	m := &monitorModel{artifactWorkspaceExpand: map[string]bool{}}
	groups := []protocol.ArtifactNode{
		{NodeKey: "day:2026-02-08", Kind: "day", Label: "2026-02-08", DayBucket: "2026-02-08"},
		{NodeKey: "role:2026-02-08:ceo", Kind: "role", Label: "ceo", DayBucket: "2026-02-08", Role: "ceo"},
		{NodeKey: "stream:2026-02-08:ceo:callback", Kind: "stream", Label: "Callback Tasks", DayBucket: "2026-02-08", Role: "ceo", TaskKind: "callback"},
		{NodeKey: "task:callback-task-ceo-1", Kind: "task", Label: "callback-task-ceo-1", DayBucket: "2026-02-08", Role: "ceo", TaskKind: "callback", TaskID: "callback-task-ceo-1", Status: "succeeded"},
		{NodeKey: "file:/workspace/deliverables/2026-02-08/callback-task-ceo-1/SUMMARY.md", Kind: "file", Label: "SUMMARY.md", VPath: "/workspace/deliverables/2026-02-08/callback-task-ceo-1/SUMMARY.md", IsSummary: true},
		{NodeKey: "file:/workspace/deliverables/2026-02-08/callback-task-ceo-1/report.md", Kind: "file", Label: "report.md", VPath: "/workspace/deliverables/2026-02-08/callback-task-ceo-1/report.md"},
	}

	tree := m.buildArtifactTreeFromGroups(groups)
	if len(tree) != 3 {
		t.Fatalf("expected role header + 2 files, got %d", len(tree))
	}
	if !tree[0].isRoleHeader || tree[0].name != "ceo" || tree[0].depth != 0 {
		t.Fatalf("expected role header first at depth 0, got %+v", tree[0])
	}
	if tree[1].name != "SUMMARY.md" || !tree[1].isSummary || tree[1].depth != 1 {
		t.Fatalf("expected summary file leaf at depth 1, got %+v", tree[1])
	}
}

func TestRebuildTree_PreservesSelectionOnExpandCollapse(t *testing.T) {
	m := &monitorModel{artifactWorkspaceExpand: map[string]bool{}}
	groups := []protocol.ArtifactNode{
		{NodeKey: "day:2026-02-08", Kind: "day", Label: "2026-02-08", DayBucket: "2026-02-08"},
		{NodeKey: "role:2026-02-08:ceo", Kind: "role", Label: "ceo", DayBucket: "2026-02-08", Role: "ceo"},
		{NodeKey: "stream:2026-02-08:ceo:task", Kind: "stream", Label: "Tasks", DayBucket: "2026-02-08", Role: "ceo", TaskKind: "task"},
		{NodeKey: "task:task-1", Kind: "task", Label: "task-1", DayBucket: "2026-02-08", Role: "ceo", TaskKind: "task", TaskID: "task-1", Status: "succeeded"},
		{NodeKey: "file:/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", Kind: "file", Label: "SUMMARY.md", VPath: "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", IsSummary: true},
	}
	m.artifactAllTree = m.buildArtifactTreeFromGroups(groups)
	m.applyArtifactVisibilityAndSearch()

	idx := m.findArtifactNodeByKey("roleflat:ceo")
	if idx < 0 {
		t.Fatalf("expected role node")
	}
	m.artifactSelected = idx
	m.artifactWorkspaceExpand["roleflat:ceo"] = true
	m.rebuildArtifactTreeWithAnchor("roleflat:ceo", idx)
	if m.artifactTree[m.artifactSelected].key != "roleflat:ceo" {
		t.Fatalf("selection moved unexpectedly to %q", m.artifactTree[m.artifactSelected].key)
	}

	m.artifactWorkspaceExpand["roleflat:ceo"] = false
	m.rebuildArtifactTreeWithAnchor("roleflat:ceo", m.artifactSelected)
	if m.artifactTree[m.artifactSelected].key != "roleflat:ceo" {
		t.Fatalf("selection should stay on collapsed node, got %q", m.artifactTree[m.artifactSelected].key)
	}
}

func TestSearchFilter_FindsInCollapsedBranches(t *testing.T) {
	m := &monitorModel{artifactWorkspaceExpand: map[string]bool{}}
	groups := []protocol.ArtifactNode{
		{NodeKey: "day:2026-02-08", Kind: "day", Label: "2026-02-08", DayBucket: "2026-02-08"},
		{NodeKey: "role:2026-02-08:ceo", Kind: "role", Label: "ceo", DayBucket: "2026-02-08", Role: "ceo"},
		{NodeKey: "stream:2026-02-08:ceo:task", Kind: "stream", Label: "Tasks", DayBucket: "2026-02-08", Role: "ceo", TaskKind: "task"},
		{NodeKey: "task:task-1", Kind: "task", Label: "task-1", DayBucket: "2026-02-08", Role: "ceo", TaskKind: "task", TaskID: "task-1", Status: "succeeded"},
		{NodeKey: "file:/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", Kind: "file", Label: "SUMMARY.md", VPath: "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", IsSummary: true},
		{NodeKey: "file:/workspace/deliverables/2026-02-08/task-1/report.md", Kind: "file", Label: "report.md", VPath: "/workspace/deliverables/2026-02-08/task-1/report.md"},
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

func TestBuildWorkspaceGroupedNodes_FlatAndSkipsDeliverables(t *testing.T) {
	entries := []workspaceEntry{
		{role: "researcher", fileLabel: "deliverables/2026-02-08/task-1/SUMMARY.md", vpath: "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md"},
		{role: "researcher", fileLabel: "researcher/report.md", vpath: "/workspace/researcher/report.md"},
		{role: "researcher", fileLabel: "researcher/data/findings.json", vpath: "/workspace/researcher/data/findings.json"},
	}

	nodes := buildWorkspaceGroupedNodes("researcher", entries)
	if len(nodes) != 3 {
		t.Fatalf("expected 1 header + 2 files, got %d: %+v", len(nodes), nodes)
	}
	if !nodes[0].isHeader || !nodes[0].isWorkspaceGroup || nodes[0].depth != 1 {
		t.Fatalf("expected workspace group header at depth 1, got %+v", nodes[0])
	}
	if nodes[1].depth != 2 || nodes[2].depth != 2 {
		t.Fatalf("expected flat file depth=2, got %+v", nodes)
	}
	for _, n := range nodes {
		if strings.Contains(n.vpath, "/workspace/deliverables/") {
			t.Fatalf("deliverables path should be skipped from workspace group: %+v", n)
		}
	}
}

func TestBuildWorkspaceGroupedNodes_NoHeaderWhenAllFiltered(t *testing.T) {
	entries := []workspaceEntry{
		{role: "shared", fileLabel: "deliverables/2026-02-08/task-1/SUMMARY.md", vpath: "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md"},
	}
	nodes := buildWorkspaceGroupedNodes("shared", entries)
	if len(nodes) != 0 {
		t.Fatalf("expected no nodes when all entries are filtered, got %+v", nodes)
	}
}

func TestInferWorkspaceRole_FromRoleDirectoryAndFallback(t *testing.T) {
	knownRoles := map[string]struct{}{
		"researcher": {},
		"writer":     {},
	}

	if got := inferWorkspaceRole("researcher/report.md", knownRoles); got != "researcher" {
		t.Fatalf("expected role from first segment, got %q", got)
	}
	if got := inferWorkspaceRole("misc/report.md", knownRoles); got != artifactWorkspaceSharedGroup {
		t.Fatalf("expected unknown segment to fallback to shared, got %q", got)
	}
	if got := inferWorkspaceRole("deliverables/2026-02-08/task-1/SUMMARY.md", knownRoles); got != artifactWorkspaceSharedGroup {
		t.Fatalf("expected legacy deliverables to fallback to shared, got %q", got)
	}
}

func TestArtifactLayout_Invariants(t *testing.T) {
	frameW, frameH := defaultMonitorStyles().panel.GetFrameSize()
	const gapCols = 1
	widths := []int{40, 60, 80, 120, 200}
	for _, w := range widths {
		navW, contentW, bodyH := artifactLayout(w, 40, frameW, frameH)
		if navW+contentW+(2*frameW)+gapCols != w {
			t.Fatalf("width invariant failed for w=%d: nav=%d content=%d frameW=%d", w, navW, contentW, frameW)
		}
		wantBodyH := max(1, 40-2-frameH)
		if bodyH != wantBodyH {
			t.Fatalf("bodyH for w=%d = %d, want %d", w, bodyH, wantBodyH)
		}
	}
}

func TestArtifactLayout_VeryNarrowStillNonNegative(t *testing.T) {
	frameW, frameH := defaultMonitorStyles().panel.GetFrameSize()
	for _, w := range []int{8, 12, 16} {
		navW, contentW, bodyH := artifactLayout(w, 12, frameW, frameH)
		if navW < 1 || contentW < 1 {
			t.Fatalf("narrow layout invalid for w=%d: nav=%d content=%d", w, navW, contentW)
		}
		if bodyH < 1 {
			t.Fatalf("narrow layout bodyH invalid for w=%d: bodyH=%d", w, bodyH)
		}
	}
}

func TestRefreshArtifactViewport_ReservesTitleRow(t *testing.T) {
	m := &monitorModel{
		styles:            defaultMonitorStyles(),
		width:             120,
		height:            40,
		artifactContentVP: viewportWithMouseDisabled(viewport.New(0, 0)),
	}
	frameW, frameH := m.styles.panel.GetFrameSize()
	_, _, bodyH := artifactLayout(m.width, m.height, frameW, frameH)
	m.refreshArtifactViewport()
	if got, want := m.artifactContentVP.Height, max(1, bodyH-1); got != want {
		t.Fatalf("artifactContentVP.Height=%d, want=%d", got, want)
	}
}

func TestRenderArtifactNavBody_TaskHeaderUsesDynamicWidth(t *testing.T) {
	m := &monitorModel{
		styles:           defaultMonitorStyles(),
		artifactSelected: -1,
		artifactTree: []artifactTreeNode{
			{
				isHeader:     true,
				isTaskHeader: true,
				expanded:     true,
				depth:        1,
				goal:         "This is a very long task goal that must be truncated by width",
				taskID:       "task-1",
				status:       "succeeded",
			},
		},
	}
	line := m.renderArtifactNavBody(1, 20)
	if got := lipgloss.Width(line); got > 20 {
		t.Fatalf("nav line width=%d exceeds max width 20: %q", got, line)
	}
	if !strings.Contains(line, taskStatusMark("succeeded")) {
		t.Fatalf("expected status mark to be preserved, got %q", line)
	}
}
