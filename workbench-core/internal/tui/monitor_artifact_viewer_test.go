package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/types"
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

func TestBuildArtifactTree_WorkspaceOnly(t *testing.T) {
	m := &monitorModel{artifactWorkspaceExpand: map[string]bool{}}
	wsFiles := []artifactTreeNode{
		{key: "wsgroup:alpha", name: "alpha", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 1},
		{key: "wsfile:/workspace/a.txt", vpath: "/workspace/a.txt", name: "a.txt", depth: 2},
	}
	tasks := []types.Task{{TaskID: "task-1", Goal: "Should not render"}}
	tree := m.buildArtifactTree(tasks, wsFiles)
	if len(tree) != 3 {
		t.Fatalf("expected exactly workspace section + group + file (3 nodes), got %d", len(tree))
	}
	if !tree[0].isWSHeader {
		t.Fatalf("expected workspace section header first, got %+v", tree[0])
	}
	for _, n := range tree {
		if n.isTaskHeader || n.isRoleHeader || strings.HasPrefix(n.key, "task:") || strings.HasPrefix(n.key, "role:") {
			t.Fatalf("unexpected deliverables node in workspace-first tree: %+v", n)
		}
	}
}

func TestWorkspaceScan_TeamRootOnly(t *testing.T) {
	tmp := t.TempDir()
	teamFile := filepath.Join(tmp, "teams", "team-1", "workspace", "team-only.txt")
	runFile := filepath.Join(tmp, "agents", "run-1", "workspace", "run-only.txt")
	if err := os.MkdirAll(filepath.Dir(teamFile), 0755); err != nil {
		t.Fatalf("mkdir team dir: %v", err)
	}
	if err := os.WriteFile(teamFile, []byte("ok"), 0644); err != nil {
		t.Fatalf("write team file: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(runFile), 0755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.WriteFile(runFile, []byte("ok"), 0644); err != nil {
		t.Fatalf("write run file: %v", err)
	}

	nodes := scanArtifactWorkspaceFiles(tmp, "team-1", "run-1", []string{"run-1"}, nil, nil)
	all := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if strings.TrimSpace(n.vpath) != "" {
			all = append(all, n.vpath)
		}
	}
	joined := strings.Join(all, "\n")
	if !strings.Contains(joined, "/workspace/team-only.txt") {
		t.Fatalf("expected team workspace file in scan, got %q", joined)
	}
	if strings.Contains(joined, "run-only.txt") {
		t.Fatalf("expected run workspace file to be excluded in team mode, got %q", joined)
	}
}

func TestWorkspaceScan_TeamGroupsByRoleAndShared(t *testing.T) {
	tmp := t.TempDir()
	p1 := filepath.Join(tmp, "teams", "team-1", "workspace", "deliverables", "2026-02-08", "task-1", "SUMMARY.md")
	p2 := filepath.Join(tmp, "teams", "team-1", "workspace", "notes.txt")
	if err := os.MkdirAll(filepath.Dir(p1), 0755); err != nil {
		t.Fatalf("mkdir p1: %v", err)
	}
	if err := os.WriteFile(p1, []byte("ok"), 0644); err != nil {
		t.Fatalf("write p1: %v", err)
	}
	if err := os.WriteFile(p2, []byte("ok"), 0644); err != nil {
		t.Fatalf("write p2: %v", err)
	}
	tasks := []types.Task{{TaskID: "task-1", AssignedRole: "analyst"}}
	nodes := scanArtifactWorkspaceFiles(tmp, "team-1", "", nil, tasks, nil)
	foundAnalyst := false
	foundShared := false
	for _, n := range nodes {
		if n.isWorkspaceGroup && n.name == "analyst" {
			foundAnalyst = true
		}
		if n.isWorkspaceGroup && n.name == artifactWorkspaceSharedGroup {
			foundShared = true
		}
	}
	if !foundAnalyst || !foundShared {
		t.Fatalf("expected analyst and shared workspace groups, got %+v", nodes)
	}
}

func TestWorkspaceScan_TeamGroupsByDayAndCategory(t *testing.T) {
	tmp := t.TempDir()
	p1 := filepath.Join(tmp, "teams", "team-1", "workspace", "deliverables", "2026-02-08", "callback-task-ceo-20260208", "SUMMARY.md")
	p2 := filepath.Join(tmp, "teams", "team-1", "workspace", "deliverables", "2026-02-08", "heartbeat-ceo-run-123", "SUMMARY.md")
	for _, p := range []string{p1, p2} {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatalf("mkdir path: %v", err)
		}
		if err := os.WriteFile(p, []byte("ok"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	nodes := scanArtifactWorkspaceFiles(tmp, "team-1", "", nil, nil, nil)
	hasDay := false
	hasCallback := false
	hasHeartbeat := false
	for _, n := range nodes {
		if !n.isWorkspaceGroup {
			continue
		}
		if n.depth == 2 && n.name == "2026-02-08" {
			hasDay = true
		}
		if n.depth == 3 && n.name == "Callback Tasks" {
			hasCallback = true
		}
		if n.depth == 3 && n.name == "Heartbeat Tasks" {
			hasHeartbeat = true
		}
	}
	if !hasDay || !hasCallback || !hasHeartbeat {
		t.Fatalf("expected day+callback+heartbeat groups, got %+v", nodes)
	}
}

func TestWorkspaceEntry_UsesCompactFileLabel(t *testing.T) {
	e := buildWorkspaceEntry("deliverables/2026-02-08/task-1/reports/final.md", "analyst", "")
	if e.fileLabel != "reports/final.md" {
		t.Fatalf("expected compact file label, got %q", e.fileLabel)
	}
}

func TestRebuildTree_PreservesSelectionOnExpandCollapse(t *testing.T) {
	m := &monitorModel{artifactWorkspaceExpand: map[string]bool{}}
	wsFiles := []artifactTreeNode{
		{key: "wsgroup:analyst", name: "analyst", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 1},
		{key: "wsfile:/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", vpath: "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", name: "deliverables/2026-02-08/task-1/SUMMARY.md", depth: 2},
	}
	m.artifactWorkspaceFiles = wsFiles
	m.artifactTree = m.buildArtifactTree(nil, wsFiles)
	idx := m.findArtifactNodeByKey("wsgroup:analyst")
	if idx < 0 {
		t.Fatalf("expected wsgroup:analyst node")
	}
	m.artifactSelected = idx
	m.artifactWorkspaceExpand["wsgroup:analyst"] = false
	m.rebuildArtifactTreeWithAnchor("wsgroup:analyst", idx)
	if m.artifactSelected < 0 || m.artifactSelected >= len(m.artifactTree) {
		t.Fatalf("selected index out of range: %d", m.artifactSelected)
	}
	if m.artifactTree[m.artifactSelected].key != "wsgroup:analyst" {
		t.Fatalf("expected selection to stay on toggled workspace group, got %q", m.artifactTree[m.artifactSelected].key)
	}
}

func TestRebuildTree_FallbackNearestWhenAnchorMissing(t *testing.T) {
	m := &monitorModel{artifactWorkspaceExpand: map[string]bool{}}
	wsFiles := []artifactTreeNode{
		{key: "wsgroup:analyst", name: "analyst", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 1},
		{key: "wsfile:/workspace/deliverables/2026-02-08/task-1/report.md", vpath: "/workspace/deliverables/2026-02-08/task-1/report.md", name: "deliverables/2026-02-08/task-1/report.md", depth: 2},
	}
	m.artifactWorkspaceFiles = wsFiles
	m.artifactTree = m.buildArtifactTree(nil, wsFiles)
	fileIdx := m.findArtifactNodeByKey("wsfile:/workspace/deliverables/2026-02-08/task-1/report.md")
	if fileIdx < 0 {
		t.Fatalf("expected ws file node")
	}
	m.artifactWorkspaceExpand["wsgroup:analyst"] = false
	m.rebuildArtifactTreeWithAnchor("wsfile:/workspace/deliverables/2026-02-08/task-1/report.md", fileIdx)
	if m.artifactSelected != 1 {
		t.Fatalf("expected fallback selection index 1 (nearest previous), got %d", m.artifactSelected)
	}
	if m.artifactTree[m.artifactSelected].key != "wsgroup:analyst" {
		t.Fatalf("expected fallback to workspace group header, got %q", m.artifactTree[m.artifactSelected].key)
	}
}

func TestRebuildTree_SearchFiltersTree(t *testing.T) {
	m := &monitorModel{
		artifactWorkspaceExpand: map[string]bool{"wsgroup:analyst": true, "wsday:analyst:2026-02-08": true, "wscat:analyst:2026-02-08:task": true, "wstask:analyst:2026-02-08:task:task-1": true},
		artifactSearchQuery:     "summary",
	}
	wsFiles := []artifactTreeNode{
		{key: "wsgroup:analyst", name: "analyst", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 1},
		{key: "wsday:analyst:2026-02-08", name: "2026-02-08", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 2},
		{key: "wscat:analyst:2026-02-08:task", name: "Tasks", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 3},
		{key: "wstask:analyst:2026-02-08:task:task-1", name: "task-1", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 4},
		{key: "wsfile:/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", vpath: "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", name: "SUMMARY.md", depth: 5},
		{key: "wsfile:/workspace/deliverables/2026-02-08/task-1/report.md", vpath: "/workspace/deliverables/2026-02-08/task-1/report.md", name: "report.md", depth: 5},
	}
	m.artifactWorkspaceFiles = wsFiles
	m.rebuildArtifactTreeWithAnchor("", 0)
	for _, n := range m.artifactTree {
		if !n.isHeader && !n.isWSHeader && !strings.Contains(strings.ToLower(n.name), "summary") {
			t.Fatalf("expected only summary file leaf after filter, got %+v", n)
		}
	}
}

func TestAppendWorkspaceSection_ParentCollapseHidesNestedChildren(t *testing.T) {
	wsFiles := []artifactTreeNode{
		{key: "wsgroup:ceo", name: "ceo", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 1},
		{key: "wsday:ceo:2026-02-08", name: "2026-02-08", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 2},
		{key: "wscat:ceo:2026-02-08:task", name: "Tasks", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 3},
		{key: "wstask:ceo:2026-02-08:task:task-1", name: "task-1", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 4},
		{key: "wsfile:/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", vpath: "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", name: "SUMMARY.md", depth: 5},
	}
	expand := map[string]bool{
		"wsgroup:ceo":                       false,
		"wsday:ceo:2026-02-08":              true,
		"wscat:ceo:2026-02-08:task":         true,
		"wstask:ceo:2026-02-08:task:task-1": true,
	}
	tree := appendWorkspaceSection(nil, wsFiles, expand, false)
	if len(tree) != 2 {
		t.Fatalf("expected only section + collapsed parent, got %d nodes: %+v", len(tree), tree)
	}
	if tree[1].key != "wsgroup:ceo" {
		t.Fatalf("expected role node as only visible child, got %q", tree[1].key)
	}
}

func TestSearchFilter_MatchesHeaderAndIncludesDescendants(t *testing.T) {
	m := &monitorModel{
		artifactWorkspaceExpand: map[string]bool{
			"wsgroup:ceo":                            true,
			"wsday:ceo:2026-02-08":                   true,
			"wscat:ceo:2026-02-08:callback":          true,
			"wstask:ceo:2026-02-08:callback:cto-001": true,
		},
		artifactSearchQuery: "callback",
	}
	wsFiles := []artifactTreeNode{
		{key: "wsgroup:ceo", name: "ceo", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 1},
		{key: "wsday:ceo:2026-02-08", name: "2026-02-08", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 2},
		{key: "wscat:ceo:2026-02-08:callback", name: "Callback Tasks", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 3},
		{key: "wstask:ceo:2026-02-08:callback:cto-001", name: "cto-001", isHeader: true, isWorkspaceGroup: true, expanded: true, depth: 4},
		{key: "wsfile:/workspace/deliverables/2026-02-08/callback-task-cto-001/SUMMARY.md", vpath: "/workspace/deliverables/2026-02-08/callback-task-cto-001/SUMMARY.md", name: "SUMMARY.md", depth: 5},
	}
	m.artifactWorkspaceFiles = wsFiles
	m.rebuildArtifactTreeWithAnchor("", 0)
	if len(m.artifactTree) == 0 {
		t.Fatalf("expected filtered tree to keep callback branch")
	}
	foundCategory := false
	foundFile := false
	for _, n := range m.artifactTree {
		if n.key == "wscat:ceo:2026-02-08:callback" {
			foundCategory = true
		}
		if strings.HasPrefix(n.key, "wsfile:") {
			foundFile = true
		}
	}
	if !foundCategory || !foundFile {
		t.Fatalf("expected callback header and descendant file to remain, got %+v", m.artifactTree)
	}
}

func TestSearchFilter_FindsMatchesInCollapsedBranches(t *testing.T) {
	m := &monitorModel{
		artifactWorkspaceExpand: map[string]bool{
			"wsgroup:ceo":                       false,
			"wsday:ceo:2026-02-08":              false,
			"wscat:ceo:2026-02-08:task":         false,
			"wstask:ceo:2026-02-08:task:task-1": false,
		},
		artifactSearchQuery: "summary",
	}
	wsFiles := []artifactTreeNode{
		{key: "wsgroup:ceo", name: "ceo", isHeader: true, isWorkspaceGroup: true, expanded: false, depth: 1},
		{key: "wsday:ceo:2026-02-08", name: "2026-02-08", isHeader: true, isWorkspaceGroup: true, expanded: false, depth: 2},
		{key: "wscat:ceo:2026-02-08:task", name: "Tasks", isHeader: true, isWorkspaceGroup: true, expanded: false, depth: 3},
		{key: "wstask:ceo:2026-02-08:task:task-1", name: "task-1", isHeader: true, isWorkspaceGroup: true, expanded: false, depth: 4},
		{key: "wsfile:/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", vpath: "/workspace/deliverables/2026-02-08/task-1/SUMMARY.md", name: "SUMMARY.md", depth: 5},
	}
	m.artifactWorkspaceFiles = wsFiles
	m.rebuildArtifactTreeWithAnchor("", 0)

	if len(m.artifactTree) == 0 {
		t.Fatalf("expected non-empty filtered tree")
	}
	foundSummary := false
	for _, n := range m.artifactTree {
		if strings.HasPrefix(n.key, "wsfile:") && strings.Contains(strings.ToLower(n.name), "summary") {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Fatalf("expected search to find SUMMARY in collapsed branch, got %+v", m.artifactTree)
	}
}
