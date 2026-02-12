package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/pkg/config"
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

func TestBuildArtifactTreeFromGroups_TwoSections(t *testing.T) {
	m := &monitorModel{artifactWorkspaceExpand: map[string]bool{}}
	groups := []protocol.ArtifactNode{
		{NodeKey: "day:2026-02-08", Kind: "day", Label: "2026-02-08", DayBucket: "2026-02-08"},
		{NodeKey: "role:2026-02-08:ceo", Kind: "role", Label: "ceo", DayBucket: "2026-02-08", Role: "ceo"},
		{NodeKey: "stream:2026-02-08:ceo:callback", Kind: "stream", Label: "Callback Tasks", DayBucket: "2026-02-08", Role: "ceo", TaskKind: "callback"},
		{NodeKey: "task:callback-task-ceo-1", Kind: "task", Label: "callback-task-ceo-1", DayBucket: "2026-02-08", Role: "ceo", TaskKind: "callback", TaskID: "callback-task-ceo-1", Status: "succeeded"},
		{NodeKey: "file:/workspace/deliverables/2026-02-08/callback-task-ceo-1/SUMMARY.md", Kind: "file", Label: "SUMMARY.md", VPath: "/workspace/deliverables/2026-02-08/callback-task-ceo-1/SUMMARY.md", TaskID: "callback-task-ceo-1", IsSummary: true},
		{NodeKey: "file:/workspace/deliverables/2026-02-08/callback-task-ceo-1/report.md", Kind: "file", Label: "report.md", VPath: "/workspace/deliverables/2026-02-08/callback-task-ceo-1/report.md"},
	}

	tree := m.buildArtifactTreeFromGroups(groups)
	if len(tree) != 6 {
		t.Fatalf("expected two sections with role/task/file nodes, got %d", len(tree))
	}
	if !tree[0].isSectionHeader || tree[0].key != "section:tasks" {
		t.Fatalf("expected tasks section first, got %+v", tree[0])
	}
	if !tree[1].isRoleHeader || tree[1].key != "tasks:role:ceo" || tree[1].depth != 1 {
		t.Fatalf("expected tasks role header at depth 1, got %+v", tree[1])
	}
	if tree[2].key != "task:callback-task-ceo-1" || tree[2].depth != 2 {
		t.Fatalf("expected task leaf at depth 2, got %+v", tree[2])
	}
	if !tree[3].isSectionHeader || tree[3].key != "section:deliverables" {
		t.Fatalf("expected deliverables section, got %+v", tree[3])
	}
	if tree[5].name != "report.md" || tree[5].depth != 2 {
		t.Fatalf("expected deliverable file at depth 2, got %+v", tree[5])
	}
	if got := m.artifactTaskSummaryMap["callback-task-ceo-1"]; got != "/workspace/deliverables/2026-02-08/callback-task-ceo-1/SUMMARY.md" {
		t.Fatalf("expected summary map entry, got %q", got)
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

	idx := m.findArtifactNodeByKey("tasks:role:ceo")
	if idx < 0 {
		t.Fatalf("expected role node")
	}
	m.artifactSelected = idx
	m.artifactWorkspaceExpand["tasks:role:ceo"] = true
	m.rebuildArtifactTreeWithAnchor("tasks:role:ceo", idx)
	if m.artifactTree[m.artifactSelected].key != "tasks:role:ceo" {
		t.Fatalf("selection moved unexpectedly to %q", m.artifactTree[m.artifactSelected].key)
	}

	m.artifactWorkspaceExpand["tasks:role:ceo"] = false
	m.rebuildArtifactTreeWithAnchor("tasks:role:ceo", m.artifactSelected)
	if m.artifactTree[m.artifactSelected].key != "tasks:role:ceo" {
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
	// Collapse all role headers.
	for _, n := range m.artifactAllTree {
		if n.isHeader && !n.isSectionHeader {
			m.artifactWorkspaceExpand[n.key] = false
		}
	}
	m.artifactSearchQuery = "task-1"
	m.applyArtifactVisibilityAndSearch()
	if len(m.artifactTree) == 0 {
		t.Fatalf("expected non-empty filtered tree")
	}
	found := false
	for _, n := range m.artifactTree {
		if strings.HasPrefix(n.key, "task:") && strings.EqualFold(n.taskID, "task-1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to find task in collapsed branches, got %+v", m.artifactTree)
	}
}

func TestSelectArtifactTreeNode_TaskLoadsSummaryPath(t *testing.T) {
	m := &monitorModel{
		artifactTaskSummaryMap: map[string]string{"task-1": "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md"},
		artifactTree:           []artifactTreeNode{{key: "task:task-1", taskID: "task-1"}},
		artifactSelected:       0,
		artifactContentVP:      viewportWithMouseDisabled(viewport.New(0, 0)),
	}
	cmd := m.selectArtifactTreeNode()
	if cmd == nil {
		t.Fatalf("expected load command for task summary")
	}
	if got := m.artifactSelectedVPath; got != "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md" {
		t.Fatalf("unexpected selected summary path %q", got)
	}
}

func TestSelectArtifactTreeNode_TaskWithoutSummaryShowsFallback(t *testing.T) {
	m := &monitorModel{
		artifactTaskSummaryMap: map[string]string{},
		artifactTree:           []artifactTreeNode{{key: "task:task-1", taskID: "task-1"}},
		artifactSelected:       0,
		artifactContentVP:      viewportWithMouseDisabled(viewport.New(0, 0)),
	}
	cmd := m.selectArtifactTreeNode()
	if cmd != nil {
		t.Fatalf("expected no command when summary is missing")
	}
	if got := m.artifactContent; got != "No summary available for this task." {
		t.Fatalf("unexpected fallback content %q", got)
	}
}

func TestUpdateArtifactViewer_NavigationSkipsSectionHeaders(t *testing.T) {
	m := &monitorModel{
		artifactNavFocused: true,
		artifactTree: []artifactTreeNode{
			{key: "section:tasks", isHeader: true, isSectionHeader: true, expanded: true},
			{key: "tasks:role:ceo", isHeader: true, isRoleHeader: true, expanded: true},
			{key: "task:task-1", taskID: "task-1"},
		},
		artifactSelected: 0,
	}
	_, _ = m.updateArtifactViewer(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := m.artifactTree[m.artifactSelected].key; got != "tasks:role:ceo" {
		t.Fatalf("expected navigation to skip section header and land on role, got %q", got)
	}
	_, _ = m.updateArtifactViewer(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if got := m.artifactTree[m.artifactSelected].key; got != "tasks:role:ceo" {
		t.Fatalf("expected navigation up to stay off section header, got %q", got)
	}
}

func TestCollapseSelectedArtifactGroup_FromLeafCollapsesParentRole(t *testing.T) {
	m := &monitorModel{
		artifactWorkspaceExpand: map[string]bool{"tasks:role:ceo": true},
		artifactTree: []artifactTreeNode{
			{key: "section:tasks", isHeader: true, isSectionHeader: true, expanded: true, depth: 0},
			{key: "tasks:role:ceo", isHeader: true, isRoleHeader: true, expanded: true, depth: 1},
			{key: "task:task-1", taskID: "task-1", depth: 2},
		},
		artifactSelected: 2,
	}
	m.collapseSelectedArtifactGroup()
	if got := m.artifactWorkspaceExpand["tasks:role:ceo"]; got {
		t.Fatalf("expected parent role header to collapse")
	}
}

func TestSearchFilter_FindsMatchesAcrossTaskAndDeliverableSections(t *testing.T) {
	m := &monitorModel{artifactWorkspaceExpand: map[string]bool{}}
	groups := []protocol.ArtifactNode{
		{NodeKey: "role:2026-02-08:ceo", Kind: "role", Label: "ceo", DayBucket: "2026-02-08", Role: "ceo"},
		{NodeKey: "task:task-1", Kind: "task", Label: "Analyze market trends", DayBucket: "2026-02-08", Role: "ceo", TaskKind: "task", TaskID: "task-1", Status: "succeeded"},
		{NodeKey: "file:/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", Kind: "file", Label: "SUMMARY.md", VPath: "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", Role: "ceo", TaskID: "task-1", IsSummary: true},
		{NodeKey: "file:/workspace/deliverables/2026-02-08/task-1/report.md", Kind: "file", Label: "report.md", VPath: "/workspace/deliverables/2026-02-08/task-1/report.md", Role: "ceo", TaskID: "task-1"},
	}
	m.artifactAllTree = m.buildArtifactTreeFromGroups(groups)
	for _, n := range m.artifactAllTree {
		if n.isRoleHeader {
			m.artifactWorkspaceExpand[n.key] = false
		}
	}

	m.artifactSearchQuery = "market trends"
	m.applyArtifactVisibilityAndSearch()
	foundTask := false
	for _, n := range m.artifactTree {
		if n.key == "task:task-1" {
			foundTask = true
			break
		}
	}
	if !foundTask {
		t.Fatalf("expected task match in search results, got %+v", m.artifactTree)
	}

	m.artifactSearchQuery = "report.md"
	m.applyArtifactVisibilityAndSearch()
	foundFile := false
	for _, n := range m.artifactTree {
		if n.key == "file:/workspace/deliverables/2026-02-08/task-1/report.md" {
			foundFile = true
			break
		}
	}
	if !foundFile {
		t.Fatalf("expected deliverable match in search results, got %+v", m.artifactTree)
	}
}

func TestHandleArtifactTreeLoaded_PrefersFirstTaskSelection(t *testing.T) {
	m := &monitorModel{
		artifactContentVP: viewportWithMouseDisabled(viewport.New(0, 0)),
	}
	msg := artifactTreeLoadedMsg{
		nodes: []protocol.ArtifactNode{
			{NodeKey: "role:2026-02-08:ceo", Kind: "role", Label: "ceo", DayBucket: "2026-02-08", Role: "ceo"},
			{NodeKey: "task:task-1", Kind: "task", Label: "Analyze market trends", DayBucket: "2026-02-08", Role: "ceo", TaskKind: "task", TaskID: "task-1", Status: "succeeded"},
			{NodeKey: "file:/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", Kind: "file", Label: "SUMMARY.md", VPath: "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", Role: "ceo", TaskID: "task-1", IsSummary: true},
			{NodeKey: "file:/workspace/deliverables/2026-02-08/task-1/report.md", Kind: "file", Label: "report.md", VPath: "/workspace/deliverables/2026-02-08/task-1/report.md", Role: "ceo", TaskID: "task-1"},
		},
	}

	cmd := m.handleArtifactTreeLoaded(msg)
	if cmd == nil {
		t.Fatalf("expected summary load command from first task selection")
	}
	if got := m.artifactTree[m.artifactSelected].key; got != "task:task-1" {
		t.Fatalf("expected first selected node to be task, got %q", got)
	}
	if got := m.artifactSelectedVPath; got != "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md" {
		t.Fatalf("expected selected summary path, got %q", got)
	}
}

func TestBuildWorkspaceGroupedNodes_FlatAndSkipsDeliverables(t *testing.T) {
	entries := []workspaceEntry{
		{role: "researcher", fileLabel: "plan/CHECKLIST.md", vpath: "/workspace/plan/CHECKLIST.md"},
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
		if strings.Contains(n.vpath, "/workspace/plan/") {
			t.Fatalf("plan path should be skipped from workspace group: %+v", n)
		}
	}
}

func TestBuildWorkspaceGroupedNodes_NoHeaderWhenAllFiltered(t *testing.T) {
	entries := []workspaceEntry{
		{role: "shared", fileLabel: "plan/CHECKLIST.md", vpath: "/workspace/plan/CHECKLIST.md"},
	}
	nodes := buildWorkspaceGroupedNodes("shared", entries)
	if len(nodes) != 0 {
		t.Fatalf("expected no nodes when all entries are filtered, got %+v", nodes)
	}
}

func TestBuildArtifactTreeFromGroups_AddsFallbackUnreportedDeliverables(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "teams", "team-1", "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "researcher"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "researcher", "report.md"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write fallback file: %v", err)
	}

	m := &monitorModel{
		cfg:                     config.Config{DataDir: dataDir},
		teamID:                  "team-1",
		artifactWorkspaceExpand: map[string]bool{},
	}
	groups := []protocol.ArtifactNode{
		{NodeKey: "role:2026-02-08:researcher", Kind: "role", Label: "researcher", DayBucket: "2026-02-08", Role: "researcher"},
		{NodeKey: "task:task-1", Kind: "task", Label: "Analyze", DayBucket: "2026-02-08", Role: "researcher", TaskKind: "task", TaskID: "task-1", Status: "succeeded"},
		{NodeKey: "file:/workspace/tasks/2026-02-08/task-1/SUMMARY.md", Kind: "file", Label: "SUMMARY.md", VPath: "/workspace/tasks/2026-02-08/task-1/SUMMARY.md", Role: "researcher", TaskID: "task-1", IsSummary: true},
	}

	tree := m.buildArtifactTreeFromGroups(groups)
	found := false
	for _, n := range tree {
		if n.vpath == "/workspace/researcher/report.md" {
			found = true
			if !n.isUnreported {
				t.Fatalf("expected fallback file to be marked unreported: %+v", n)
			}
		}
	}
	if !found {
		t.Fatalf("expected fallback unreported deliverable in tree: %+v", tree)
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
