package tui

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	"github.com/tinoosan/workbench-core/pkg/types"
)

const artifactFileReadLimitBytes = 256 * 1024

const artifactWorkspaceSharedGroup = "shared"

var (
	artifactStyleSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f4b8e4"))
	artifactStyleGroup   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#89dceb"))
	artifactStyleFile    = lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
)

type artifactTreeNode struct {
	key    string
	taskID string
	goal   string
	role   string
	kind   string
	day    string
	status string
	runID  string

	vpath        string
	diskPath     string
	name         string
	isSummary    bool
	isUnreported bool

	isHeader         bool
	isSectionHeader  bool
	isDayHeader      bool
	isRoleHeader     bool
	isKindHeader     bool
	isTaskHeader     bool
	isWorkspaceGroup bool
	isWSHeader       bool
	expanded         bool
	depth            int
}

type artifactTreeLoadedMsg struct {
	nodes []protocol.ArtifactNode
	err   error
}

type artifactContentLoadedMsg struct {
	vpath   string
	content string
	err     error
}

func (m *monitorModel) openArtifactViewer() tea.Cmd {
	if m == nil {
		return nil
	}
	m.artifactViewerOpen = true
	m.artifactTasks = nil
	m.artifactTree = nil
	m.artifactAllTree = nil
	m.artifactWorkspaceFiles = nil
	m.artifactTaskSummaryMap = map[string]string{}
	m.artifactSelected = 0
	m.artifactSelectedVPath = ""
	m.artifactContentRaw = ""
	m.artifactContent = "Loading artifacts..."
	m.artifactRenderWidth = 0
	m.artifactRenderRawLen = 0
	m.artifactRenderedVPath = ""
	m.artifactSearchMode = false
	m.artifactSearchQuery = ""
	m.artifactSearchScopeKey = ""
	m.artifactNavFocused = true
	m.artifactRoleExpanded = map[string]bool{}
	m.artifactTaskExpanded = map[string]bool{}
	m.artifactWorkspaceExpand = map[string]bool{}
	m.artifactContentVP = viewportWithMouseDisabled(m.artifactContentVP)
	m.refreshArtifactViewport()
	return m.loadArtifactTree()
}

func (m *monitorModel) closeArtifactViewer() {
	if m == nil {
		return
	}
	m.artifactViewerOpen = false
	m.artifactTasks = nil
	m.artifactTree = nil
	m.artifactAllTree = nil
	m.artifactWorkspaceFiles = nil
	m.artifactTaskSummaryMap = nil
	m.artifactSelected = 0
	m.artifactSelectedVPath = ""
	m.artifactContent = ""
	m.artifactContentRaw = ""
	m.artifactRenderWidth = 0
	m.artifactRenderRawLen = 0
	m.artifactRenderedVPath = ""
	m.artifactSearchMode = false
	m.artifactSearchQuery = ""
	m.artifactSearchScopeKey = ""
	m.artifactNavFocused = false
	m.artifactRoleExpanded = nil
	m.artifactTaskExpanded = nil
	m.artifactWorkspaceExpand = nil
}

func (m *monitorModel) loadArtifactTree() tea.Cmd {
	if m == nil {
		return func() tea.Msg {
			return artifactTreeLoadedMsg{err: fmt.Errorf("monitor is not available")}
		}
	}
	return func() tea.Msg {
		params := protocol.ArtifactListParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			Limit:    5000,
		}
		if strings.TrimSpace(m.teamID) != "" {
			params.TeamID = strings.TrimSpace(m.teamID)
		}
		var res protocol.ArtifactListResult
		if err := m.rpcRoundTrip(protocol.MethodArtifactList, params, &res); err != nil {
			return artifactTreeLoadedMsg{err: err}
		}
		return artifactTreeLoadedMsg{nodes: res.Nodes}
	}
}

func dedupeArtifactPaths(paths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if !strings.HasPrefix(p, "/workspace/") && !strings.HasPrefix(p, "/tasks/") {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func scanArtifactWorkspaceFiles(dataDir, teamID, singleRunID string, teamRunIDs []string, tasks []types.Task, roleByRunID map[string]string) []artifactTreeNode {
	if strings.TrimSpace(teamID) != "" {
		return scanTeamWorkspaceFiles(dataDir, teamID, tasks, roleByRunID)
	}
	return scanRunWorkspaceFiles(dataDir, singleRunID, teamRunIDs, tasks)
}

func scanRunWorkspaceFiles(dataDir, singleRunID string, teamRunIDs []string, tasks []types.Task) []artifactTreeNode {
	_ = tasks
	runSet := map[string]struct{}{}
	for _, rid := range teamRunIDs {
		rid = strings.TrimSpace(rid)
		if rid != "" {
			runSet[rid] = struct{}{}
		}
	}
	for _, task := range tasks {
		rid := strings.TrimSpace(task.RunID)
		if rid != "" {
			runSet[rid] = struct{}{}
		}
	}
	if len(runSet) == 0 && strings.TrimSpace(singleRunID) != "" {
		runSet[strings.TrimSpace(singleRunID)] = struct{}{}
	}

	runIDs := make([]string, 0, len(runSet))
	for rid := range runSet {
		runIDs = append(runIDs, rid)
	}
	sort.Strings(runIDs)

	nodes := make([]artifactTreeNode, 0, len(runIDs)*8)
	for _, rid := range runIDs {
		files := scanWorkspaceRelativeFiles(fsutil.GetWorkspaceDir(dataDir, rid), 20000)
		entries := make([]workspaceEntry, 0, len(files))
		for _, rel := range files {
			e := buildWorkspaceEntry(rel, rid, rid)
			entries = append(entries, e)
		}
		nodes = append(nodes, buildWorkspaceGroupedNodes(rid, entries)...)
	}
	return nodes
}

func scanTeamWorkspaceFiles(dataDir, teamID string, tasks []types.Task, roleByRunID map[string]string) []artifactTreeNode {
	wsDir := fsutil.GetTeamWorkspaceDir(dataDir, teamID)
	relFiles := scanWorkspaceRelativeFiles(wsDir, 50000)
	if len(relFiles) == 0 {
		return nil
	}

	knownRoles := map[string]struct{}{}
	for _, t := range tasks {
		role := strings.TrimSpace(roleForTask(t, roleByRunID))
		if role != "" {
			knownRoles[role] = struct{}{}
		}
	}

	grouped := map[string][]workspaceEntry{}
	for _, rel := range relFiles {
		role := inferWorkspaceRole(rel, knownRoles)
		grouped[role] = append(grouped[role], buildWorkspaceEntry(rel, role, ""))
	}

	roles := make([]string, 0, len(grouped))
	for role := range grouped {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	nodes := make([]artifactTreeNode, 0, len(relFiles)+len(roles)*4)
	for _, role := range roles {
		nodes = append(nodes, buildWorkspaceGroupedNodes(role, grouped[role])...)
	}
	return nodes
}

type workspaceEntry struct {
	role      string
	fileLabel string
	vpath     string
}

func buildWorkspaceEntry(rel, role, runID string) workspaceEntry {
	rel = strings.TrimSpace(filepath.ToSlash(rel))
	fileLabel := rel
	if fileLabel == "" {
		fileLabel = filepath.Base(rel)
	}
	e := workspaceEntry{
		role:      strings.TrimSpace(role),
		fileLabel: fileLabel,
		vpath:     "/workspace/" + rel,
	}
	if strings.TrimSpace(e.role) == "" {
		if strings.TrimSpace(runID) != "" {
			e.role = runID
		} else {
			e.role = artifactWorkspaceSharedGroup
		}
	}
	return e
}

func buildWorkspaceGroupedNodes(role string, entries []workspaceEntry) []artifactTreeNode {
	if len(entries) == 0 {
		return nil
	}
	filtered := make([]workspaceEntry, 0, len(entries))
	for _, e := range entries {
		rel := strings.TrimPrefix(strings.TrimSpace(e.vpath), "/workspace/")
		if strings.HasPrefix(rel, "plan/") {
			continue
		}
		filtered = append(filtered, e)
	}
	if len(filtered) == 0 {
		return nil
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].fileLabel < filtered[j].fileLabel
	})

	nodes := make([]artifactTreeNode, 0, len(filtered)+1)
	role = strings.TrimSpace(role)
	roleKey := "wsgroup:" + role
	nodes = append(nodes, artifactTreeNode{
		key:              roleKey,
		role:             role,
		name:             role,
		isHeader:         true,
		isWorkspaceGroup: true,
		expanded:         false,
		depth:            1,
	})
	for _, e := range filtered {
		nodes = append(nodes, artifactTreeNode{
			key:   "wsfile:" + e.vpath,
			role:  role,
			vpath: e.vpath,
			name:  e.fileLabel,
			depth: 2,
		})
	}
	return nodes
}

func scanWorkspaceRelativeFiles(baseDir string, maxVisited int) []string {
	if strings.TrimSpace(baseDir) == "" {
		return nil
	}
	if maxVisited <= 0 {
		maxVisited = 20000
	}
	files := make([]string, 0, 256)
	visited := 0
	_ = filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != baseDir && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		visited++
		if visited > maxVisited {
			return fs.SkipAll
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(strings.TrimSpace(rel))
		if rel == "" || rel == "." {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	sort.Strings(files)
	return files
}

func inferWorkspaceRole(rel string, knownRoles map[string]struct{}) string {
	rel = strings.TrimSpace(filepath.ToSlash(rel))
	if rel == "" {
		return artifactWorkspaceSharedGroup
	}
	parts := strings.Split(rel, "/")
	if len(parts) > 0 {
		candidate := strings.TrimSpace(parts[0])
		if candidate != "" {
			if _, ok := knownRoles[candidate]; ok {
				return candidate
			}
		}
	}
	return artifactWorkspaceSharedGroup
}

func roleForTask(task types.Task, roleByRunID map[string]string) string {
	if role := strings.TrimSpace(task.AssignedRole); role != "" {
		return role
	}
	runID := strings.TrimSpace(task.RunID)
	if runID != "" {
		if role := strings.TrimSpace(roleByRunID[runID]); role != "" {
			return role
		}
	}
	return "unassigned"
}

func taskKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case "callback":
		return "Callback Tasks"
	case "heartbeat":
		return "Heartbeat Tasks"
	case "coordinator":
		return "Coordinator Tasks"
	case "task":
		return "Tasks"
	default:
		return "Other Tasks"
	}
}

func taskStatusMark(status string) string {
	switch strings.TrimSpace(status) {
	case string(types.TaskStatusSucceeded):
		return "✓"
	case string(types.TaskStatusFailed):
		return "✗"
	case string(types.TaskStatusCanceled):
		return "∅"
	default:
		return "•"
	}
}

func (m *monitorModel) buildArtifactTreeFromGroups(nodes []protocol.ArtifactNode) []artifactTreeNode {
	if m == nil {
		return nil
	}
	if m.artifactWorkspaceExpand == nil {
		m.artifactWorkspaceExpand = map[string]bool{}
	}
	if m.artifactTaskSummaryMap == nil {
		m.artifactTaskSummaryMap = map[string]string{}
	} else {
		for k := range m.artifactTaskSummaryMap {
			delete(m.artifactTaskSummaryMap, k)
		}
	}
	if len(nodes) == 0 {
		return []artifactTreeNode{
			{key: "section:tasks", name: "TASKS", isHeader: true, isSectionHeader: true, expanded: true, depth: 0},
			{key: "section:deliverables", name: "DELIVERABLES", isHeader: true, isSectionHeader: true, expanded: true, depth: 0},
		}
	}
	taskByRole := map[string][]artifactTreeNode{}
	deliverablesByRole := map[string][]artifactTreeNode{}
	indexedVPaths := map[string]struct{}{}
	taskRoles := make([]string, 0, 8)
	deliverableRoles := make([]string, 0, 8)
	seenTaskRoles := map[string]struct{}{}
	seenDeliverableRoles := map[string]struct{}{}
	currentRole := ""
	for _, n := range nodes {
		kind := strings.TrimSpace(n.Kind)
		if kind == "role" {
			currentRole = strings.TrimSpace(n.Role)
			if currentRole == "" {
				currentRole = strings.TrimSpace(n.Label)
			}
			continue
		}
		role := strings.TrimSpace(n.Role)
		if role == "" {
			role = strings.TrimSpace(currentRole)
		}
		if role == "" {
			role = "unassigned"
		}
		switch kind {
		case "task":
			taskID := strings.TrimSpace(n.TaskID)
			if taskID == "" {
				continue
			}
			goal := strings.TrimSpace(n.Label)
			if goal == "" {
				goal = taskID
			}
			if _, ok := seenTaskRoles[role]; !ok {
				seenTaskRoles[role] = struct{}{}
				taskRoles = append(taskRoles, role)
			}
			taskByRole[role] = append(taskByRole[role], artifactTreeNode{
				key:    "task:" + taskID,
				taskID: taskID,
				goal:   goal,
				role:   role,
				kind:   strings.TrimSpace(n.TaskKind),
				day:    strings.TrimSpace(n.DayBucket),
				status: strings.TrimSpace(n.Status),
				name:   goal,
				depth:  2,
			})
		case "file":
			taskID := strings.TrimSpace(n.TaskID)
			vpath := strings.TrimSpace(n.VPath)
			if n.IsSummary {
				if taskID != "" && vpath != "" {
					if _, exists := m.artifactTaskSummaryMap[taskID]; !exists {
						m.artifactTaskSummaryMap[taskID] = vpath
					}
				}
				continue
			}
			if vpath == "" {
				continue
			}
			indexedVPaths[vpath] = struct{}{}
			name := strings.TrimSpace(n.Label)
			if name == "" {
				name = filepath.Base(vpath)
			}
			if _, ok := seenDeliverableRoles[role]; !ok {
				seenDeliverableRoles[role] = struct{}{}
				deliverableRoles = append(deliverableRoles, role)
			}
			deliverablesByRole[role] = append(deliverablesByRole[role], artifactTreeNode{
				key:       "file:" + vpath,
				taskID:    taskID,
				role:      role,
				kind:      strings.TrimSpace(n.TaskKind),
				day:       strings.TrimSpace(n.DayBucket),
				status:    strings.TrimSpace(n.Status),
				vpath:     vpath,
				diskPath:  strings.TrimSpace(n.DiskPath),
				name:      name,
				isSummary: false,
				depth:     2,
			})
		default:
			continue
		}
	}
	knownTasks := make([]types.Task, 0, len(taskRoles)+len(deliverableRoles))
	knownRoles := map[string]struct{}{}
	for _, role := range taskRoles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if _, ok := knownRoles[role]; ok {
			continue
		}
		knownRoles[role] = struct{}{}
		knownTasks = append(knownTasks, types.Task{AssignedRole: role})
	}
	for _, role := range deliverableRoles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if _, ok := knownRoles[role]; ok {
			continue
		}
		knownRoles[role] = struct{}{}
		knownTasks = append(knownTasks, types.Task{AssignedRole: role})
	}
	if strings.TrimSpace(m.cfg.DataDir) != "" {
		fallbackAdded := map[string]struct{}{}
		wsNodes := scanArtifactWorkspaceFiles(m.cfg.DataDir, m.teamID, m.runID, m.teamRunIDs, knownTasks, m.teamRoleByRunID)
		for _, ws := range wsNodes {
			if ws.isHeader || strings.TrimSpace(ws.vpath) == "" {
				continue
			}
			vpath := strings.TrimSpace(ws.vpath)
			if _, ok := indexedVPaths[vpath]; ok {
				continue
			}
			if _, ok := fallbackAdded[vpath]; ok {
				continue
			}
			name := strings.TrimSpace(ws.name)
			if name == "" {
				name = filepath.Base(vpath)
			}
			if strings.EqualFold(name, "SUMMARY.md") {
				continue
			}
			if strings.HasPrefix(vpath, "/workspace/") && strings.HasPrefix(strings.TrimPrefix(vpath, "/workspace/"), "plan/") {
				continue
			}
			role := strings.TrimSpace(ws.role)
			if role == "" {
				role = "unassigned"
			}
			if _, ok := seenDeliverableRoles[role]; !ok {
				seenDeliverableRoles[role] = struct{}{}
				deliverableRoles = append(deliverableRoles, role)
			}
			deliverablesByRole[role] = append(deliverablesByRole[role], artifactTreeNode{
				key:          "file:" + vpath,
				role:         role,
				vpath:        vpath,
				diskPath:     strings.TrimSpace(ws.diskPath),
				name:         name,
				isSummary:    false,
				isUnreported: true,
				depth:        2,
			})
			fallbackAdded[vpath] = struct{}{}
		}
	}

	tree := make([]artifactTreeNode, 0, len(nodes)+8)
	tree = append(tree, artifactTreeNode{
		key:             "section:tasks",
		name:            "TASKS",
		isHeader:        true,
		isSectionHeader: true,
		expanded:        true,
		depth:           0,
	})
	for _, role := range taskRoles {
		key := "tasks:role:" + role
		expanded := true
		if v, ok := m.artifactWorkspaceExpand[key]; ok {
			expanded = v
		} else {
			m.artifactWorkspaceExpand[key] = true
		}
		tree = append(tree, artifactTreeNode{
			key:          key,
			role:         role,
			name:         role,
			isHeader:     true,
			isRoleHeader: true,
			expanded:     expanded,
			depth:        1,
		})
		tree = append(tree, taskByRole[role]...)
	}

	tree = append(tree, artifactTreeNode{
		key:             "section:deliverables",
		name:            "DELIVERABLES",
		isHeader:        true,
		isSectionHeader: true,
		expanded:        true,
		depth:           0,
	})
	for _, role := range deliverableRoles {
		key := "deliverables:role:" + role
		expanded := true
		if v, ok := m.artifactWorkspaceExpand[key]; ok {
			expanded = v
		} else {
			m.artifactWorkspaceExpand[key] = true
		}
		tree = append(tree, artifactTreeNode{
			key:          key,
			role:         role,
			name:         role,
			isHeader:     true,
			isRoleHeader: true,
			expanded:     expanded,
			depth:        1,
		})
		tree = append(tree, deliverablesByRole[role]...)
	}
	return tree
}

func (m *monitorModel) buildSingleRunArtifactTree(tasks []types.Task, wsFiles []artifactTreeNode) []artifactTreeNode {
	tree := make([]artifactTreeNode, 0, len(tasks)*4+len(wsFiles)+2)
	for _, task := range tasks {
		taskID := strings.TrimSpace(task.TaskID)
		expanded := true
		if m.artifactTaskExpanded != nil {
			if v, ok := m.artifactTaskExpanded[taskID]; ok {
				expanded = v
			} else {
				m.artifactTaskExpanded[taskID] = true
			}
		}
		role := strings.TrimSpace(task.AssignedRole)
		header := artifactTreeNode{
			key:          "task:" + taskID,
			taskID:       taskID,
			goal:         strings.TrimSpace(task.Goal),
			role:         role,
			status:       strings.TrimSpace(string(task.Status)),
			runID:        strings.TrimSpace(task.RunID),
			isHeader:     true,
			isTaskHeader: true,
			expanded:     expanded,
			depth:        0,
		}
		tree = append(tree, header)
		if !expanded {
			continue
		}
		for _, fileNode := range artifactTaskFiles(task, 1) {
			tree = append(tree, fileNode)
		}
	}
	return appendWorkspaceSection(tree, wsFiles, m.artifactWorkspaceExpand, false)
}

func (m *monitorModel) buildTeamArtifactTree(tasks []types.Task, wsFiles []artifactTreeNode) []artifactTreeNode {
	if m.artifactRoleExpanded == nil {
		m.artifactRoleExpanded = map[string]bool{}
	}
	if m.artifactTaskExpanded == nil {
		m.artifactTaskExpanded = map[string]bool{}
	}

	grouped := map[string][]types.Task{}
	for _, task := range tasks {
		role := roleForTask(task, m.teamRoleByRunID)
		grouped[role] = append(grouped[role], task)
	}
	roles := make([]string, 0, len(grouped))
	for role := range grouped {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	tree := make([]artifactTreeNode, 0, len(tasks)*4+len(roles)+len(wsFiles)+2)
	for _, role := range roles {
		expanded := true
		if v, ok := m.artifactRoleExpanded[role]; ok {
			expanded = v
		} else {
			m.artifactRoleExpanded[role] = true
		}
		roleNode := artifactTreeNode{
			key:          "role:" + role,
			role:         role,
			name:         role,
			isHeader:     true,
			isRoleHeader: true,
			expanded:     expanded,
			depth:        0,
		}
		tree = append(tree, roleNode)
		if !expanded {
			continue
		}

		roleTasks := grouped[role]
		for _, task := range roleTasks {
			taskID := strings.TrimSpace(task.TaskID)
			taskExpanded := true
			if v, ok := m.artifactTaskExpanded[taskID]; ok {
				taskExpanded = v
			} else {
				m.artifactTaskExpanded[taskID] = true
			}
			taskNode := artifactTreeNode{
				key:          "task:" + taskID,
				taskID:       taskID,
				goal:         strings.TrimSpace(task.Goal),
				role:         role,
				status:       strings.TrimSpace(string(task.Status)),
				runID:        strings.TrimSpace(task.RunID),
				isHeader:     true,
				isTaskHeader: true,
				expanded:     taskExpanded,
				depth:        1,
			}
			tree = append(tree, taskNode)
			if !taskExpanded {
				continue
			}
			for _, fileNode := range artifactTaskFiles(task, 2) {
				fileNode.role = role
				tree = append(tree, fileNode)
			}
		}
	}

	return appendWorkspaceSection(tree, wsFiles, m.artifactWorkspaceExpand, false)
}

func artifactTaskFiles(task types.Task, depth int) []artifactTreeNode {
	artifacts := append([]string(nil), task.Artifacts...)
	sort.SliceStable(artifacts, func(i, j int) bool {
		iSummary := strings.EqualFold(filepath.Base(artifacts[i]), "SUMMARY.md")
		jSummary := strings.EqualFold(filepath.Base(artifacts[j]), "SUMMARY.md")
		if iSummary != jSummary {
			return iSummary
		}
		return artifacts[i] < artifacts[j]
	})
	out := make([]artifactTreeNode, 0, len(artifacts))
	for _, vp := range artifacts {
		vp = strings.TrimSpace(vp)
		if vp == "" {
			continue
		}
		name := filepath.Base(vp)
		out = append(out, artifactTreeNode{
			key:       "file:" + vp,
			taskID:    strings.TrimSpace(task.TaskID),
			runID:     strings.TrimSpace(task.RunID),
			vpath:     vp,
			name:      name,
			isSummary: strings.EqualFold(name, "SUMMARY.md"),
			depth:     depth,
		})
	}
	return out
}

func appendWorkspaceSection(tree, wsFiles []artifactTreeNode, expand map[string]bool, forceExpand bool) []artifactTreeNode {
	if len(wsFiles) == 0 {
		return tree
	}
	tree = append(tree, artifactTreeNode{
		key:        "ws:section",
		name:       "All Workspace Files",
		isWSHeader: true,
		depth:      0,
	})
	visibleAtDepth := map[int]bool{0: true}
	for _, n := range wsFiles {
		node := n
		if node.isWorkspaceGroup {
			parentVisible := true
			if node.depth > 0 {
				parentVisible = visibleAtDepth[node.depth-1]
			}
			if !parentVisible {
				visibleAtDepth[node.depth] = false
				continue
			}
			expanded := node.expanded
			if forceExpand {
				// During search/filtering, always traverse the full tree regardless
				// of the user's expansion state so matches are never hidden.
				expanded = true
			} else if expand != nil {
				if v, ok := expand[node.key]; ok {
					expanded = v
				} else {
					expand[node.key] = expanded
				}
			}
			node.expanded = expanded
			tree = append(tree, node)
			visibleAtDepth[node.depth] = expanded
			continue
		}
		parentVisible := true
		if node.depth > 0 {
			parentVisible = visibleAtDepth[node.depth-1]
		}
		if parentVisible {
			tree = append(tree, node)
		}
	}
	return tree
}

func (m *monitorModel) loadArtifactContent(vpath, diskPath, runID string) tea.Cmd {
	vpath = strings.TrimSpace(vpath)
	if vpath == "" {
		return nil
	}
	return func() tea.Msg {
		_ = diskPath
		_ = runID
		params := protocol.ArtifactGetParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			VPath:    vpath,
			MaxBytes: artifactFileReadLimitBytes,
		}
		if strings.TrimSpace(m.teamID) != "" {
			params.TeamID = strings.TrimSpace(m.teamID)
		}
		var res protocol.ArtifactGetResult
		if err := m.rpcRoundTrip(protocol.MethodArtifactGet, params, &res); err != nil {
			return artifactContentLoadedMsg{vpath: vpath, err: err}
		}
		content := res.Content
		if res.Truncated {
			content += "\n\n[truncated: file exceeds 256KB]"
		}
		return artifactContentLoadedMsg{vpath: vpath, content: content}
	}
}

func resolveArtifactDisk(dataDir, teamID, runID, vpath string) string {
	vpath = strings.TrimSpace(vpath)
	if vpath == "" {
		return ""
	}
	if strings.HasPrefix(vpath, "/tasks/") {
		rel := strings.TrimPrefix(vpath, "/tasks/")
		rel = strings.TrimPrefix(rel, "/")
		if strings.TrimSpace(teamID) != "" {
			return filepath.Join(fsutil.GetTeamWorkspaceDir(dataDir, teamID), "tasks", rel)
		}
		const subagentsPrefix = "subagents/"
		if strings.HasPrefix(rel, subagentsPrefix) {
			rest := strings.TrimPrefix(rel, subagentsPrefix)
			parts := strings.SplitN(rest, string(filepath.Separator), 2)
			if len(parts) >= 2 && parts[0] != "" {
				childRunID := parts[0]
				restPath := parts[1]
				base := fsutil.GetSubagentTasksDir(dataDir, runID, childRunID)
				return filepath.Join(base, restPath)
			}
		}
		return filepath.Join(fsutil.GetTasksDir(dataDir, runID), rel)
	}
	if !strings.HasPrefix(vpath, "/workspace/") {
		return ""
	}
	rel := strings.TrimPrefix(vpath, "/workspace/")
	if strings.TrimSpace(teamID) != "" {
		return filepath.Join(fsutil.GetTeamWorkspaceDir(dataDir, teamID), rel)
	}
	return filepath.Join(fsutil.GetWorkspaceDir(dataDir, runID), rel)
}

func renderArtifactByExt(name, content string, width int, renderer *ContentRenderer) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".md", ".markdown":
		if renderer != nil {
			return renderer.RenderMarkdown(content, width)
		}
		return content
	case ".json":
		if renderer != nil {
			return renderer.RenderMarkdown(FormatJSON(content), width)
		}
		return FormatJSON(content)
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".yaml", ".yml", ".sh", ".sql", ".toml":
		lang := artifactLangFromExt(ext)
		if renderer != nil {
			return renderer.RenderMarkdown(FormatCode(lang, content), width)
		}
		return FormatCode(lang, content)
	default:
		return content
	}
}

func artifactLangFromExt(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".jsx":
		return "jsx"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".yaml", ".yml":
		return "yaml"
	case ".sh":
		return "bash"
	case ".sql":
		return "sql"
	case ".toml":
		return "toml"
	default:
		return "text"
	}
}

func (m *monitorModel) updateArtifactViewer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m == nil {
		return m, nil
	}

	switch strings.ToLower(msg.String()) {
	case "esc", "escape":
		if m.artifactSearchMode {
			if strings.TrimSpace(m.artifactSearchQuery) != "" {
				m.artifactSearchQuery = ""
				key, idx := m.currentArtifactAnchor()
				m.rebuildArtifactTreeWithAnchor(key, idx)
				return m, nil
			}
			m.artifactSearchMode = false
			m.artifactSearchScopeKey = ""
			return m, nil
		}
		m.closeArtifactViewer()
		return m, nil
	case "tab":
		if m.artifactSearchMode {
			return m, nil
		}
		m.artifactNavFocused = !m.artifactNavFocused
		return m, nil
	}
	if m.artifactSearchMode {
		return m.updateArtifactSearchInput(msg)
	}

	if !m.artifactNavFocused {
		switch strings.ToLower(msg.String()) {
		case "j", "down", "ctrl+d", "pgdown", "pgdn":
			m.artifactContentVP.LineDown(1)
			return m, nil
		case "k", "up", "ctrl+u", "pgup":
			m.artifactContentVP.LineUp(1)
			return m, nil
		}
		var cmd tea.Cmd
		m.artifactContentVP, cmd = m.artifactContentVP.Update(msg)
		return m, cmd
	}

	switch strings.ToLower(msg.String()) {
	case "/", "ctrl+f":
		m.artifactSearchMode = true
		if m.artifactSelected >= 0 && m.artifactSelected < len(m.artifactTree) {
			n := m.artifactTree[m.artifactSelected]
			if n.isHeader {
				m.artifactSearchScopeKey = n.key
			} else {
				m.artifactSearchScopeKey = ""
			}
		}
		return m, nil
	case "j", "down":
		m.moveArtifactSelection(1)
		return m, nil
	case "k", "up":
		m.moveArtifactSelection(-1)
		return m, nil
	case "ctrl+d", "pgdown", "pgdn":
		m.artifactContentVP.LineDown(max(1, m.artifactContentVP.Height/2))
		return m, nil
	case "ctrl+u", "pgup":
		m.artifactContentVP.LineUp(max(1, m.artifactContentVP.Height/2))
		return m, nil
	case "h", "left":
		m.collapseSelectedArtifactGroup()
		return m, nil
	case "enter", "l", "right":
		return m, m.selectArtifactTreeNode()
	default:
		return m, nil
	}
}

func (m *monitorModel) updateArtifactSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch strings.ToLower(msg.String()) {
	case "enter":
		m.artifactSearchMode = false
		return m, nil
	case "backspace":
		if m.artifactSearchQuery != "" {
			r := []rune(m.artifactSearchQuery)
			m.artifactSearchQuery = string(r[:len(r)-1])
			key, idx := m.currentArtifactAnchor()
			m.rebuildArtifactTreeWithAnchor(key, idx)
		}
		return m, nil
	}
	if len(msg.Runes) > 0 {
		for _, r := range msg.Runes {
			if r >= 32 && r != 127 {
				m.artifactSearchQuery += string(r)
			}
		}
		key, idx := m.currentArtifactAnchor()
		m.rebuildArtifactTreeWithAnchor(key, idx)
	}
	return m, nil
}

func (m *monitorModel) selectArtifactTreeNode() tea.Cmd {
	if m == nil || m.artifactSelected < 0 || m.artifactSelected >= len(m.artifactTree) {
		return nil
	}
	node := m.artifactTree[m.artifactSelected]
	if node.isSectionHeader {
		return nil
	}
	if node.isWSHeader {
		return nil
	}
	if strings.HasPrefix(node.key, "task:") {
		taskID := strings.TrimSpace(strings.TrimPrefix(node.key, "task:"))
		if taskID == "" {
			return nil
		}
		summaryPath := strings.TrimSpace(m.artifactTaskSummaryMap[taskID])
		if summaryPath == "" {
			m.artifactSelectedVPath = ""
			m.artifactContentRaw = ""
			m.artifactContent = "No summary available for this task."
			m.artifactRenderWidth = 0
			m.artifactRenderRawLen = 0
			m.artifactRenderedVPath = ""
			m.artifactContentVP.SetContent(m.artifactContent)
			m.artifactContentVP.GotoTop()
			return nil
		}
		m.artifactSelectedVPath = summaryPath
		m.artifactContent = "Loading SUMMARY.md..."
		m.artifactContentRaw = ""
		m.artifactRenderWidth = 0
		m.artifactRenderRawLen = 0
		m.artifactRenderedVPath = ""
		m.artifactContentVP.SetContent(m.artifactContent)
		m.artifactContentVP.GotoTop()
		return m.loadArtifactContent(summaryPath, "", node.runID)
	}
	if node.isHeader {
		if m.artifactWorkspaceExpand == nil {
			m.artifactWorkspaceExpand = map[string]bool{}
		}
		m.artifactWorkspaceExpand[node.key] = !node.expanded
		m.rebuildArtifactTreeWithAnchor(node.key, m.artifactSelected)
		return nil
	}
	if strings.TrimSpace(node.vpath) == "" {
		return nil
	}
	m.artifactSelectedVPath = node.vpath
	m.artifactContent = "Loading " + node.name + "..."
	m.artifactContentRaw = ""
	m.artifactRenderWidth = 0
	m.artifactRenderRawLen = 0
	m.artifactRenderedVPath = ""
	m.artifactContentVP.SetContent(m.artifactContent)
	m.artifactContentVP.GotoTop()
	return m.loadArtifactContent(node.vpath, node.diskPath, node.runID)
}

func (m *monitorModel) collapseSelectedArtifactGroup() {
	if m == nil || m.artifactSelected < 0 || m.artifactSelected >= len(m.artifactTree) {
		return
	}
	if m.artifactWorkspaceExpand == nil {
		m.artifactWorkspaceExpand = map[string]bool{}
	}

	node := m.artifactTree[m.artifactSelected]
	if node.isSectionHeader {
		return
	}
	if node.isHeader {
		if node.expanded {
			m.artifactWorkspaceExpand[node.key] = false
			m.rebuildArtifactTreeWithAnchor(node.key, m.artifactSelected)
			return
		}
	}

	for i := m.artifactSelected - 1; i >= 0; i-- {
		parent := m.artifactTree[i]
		if !parent.isHeader {
			continue
		}
		if parent.isSectionHeader {
			break
		}
		if parent.expanded {
			m.artifactWorkspaceExpand[parent.key] = false
			m.rebuildArtifactTreeWithAnchor(parent.key, i)
		}
		break
	}
}

func (m *monitorModel) moveArtifactSelection(delta int) {
	if m == nil || len(m.artifactTree) == 0 || delta == 0 {
		return
	}
	i := m.artifactSelected
	for {
		next := i + delta
		if next < 0 || next >= len(m.artifactTree) {
			return
		}
		i = next
		if !m.artifactTree[i].isSectionHeader {
			m.artifactSelected = i
			return
		}
	}
}

func (m *monitorModel) rebuildArtifactTreeWithAnchor(anchorKey string, anchorIndex int) {
	m.applyArtifactVisibilityAndSearch()
	if len(m.artifactTree) == 0 {
		m.artifactSelected = 0
		return
	}
	if idx := m.findArtifactNodeByKey(anchorKey); idx >= 0 {
		m.artifactSelected = idx
		return
	}
	if anchorIndex < 0 {
		anchorIndex = 0
	}
	if anchorIndex >= len(m.artifactTree) {
		anchorIndex = len(m.artifactTree) - 1
	}
	m.artifactSelected = anchorIndex
}

func (m *monitorModel) applyArtifactVisibilityAndSearch() {
	if len(m.artifactAllTree) == 0 {
		m.artifactTree = nil
		return
	}
	base := make([]artifactTreeNode, 0, len(m.artifactAllTree))
	visible := map[int]bool{0: true}
	for _, node := range m.artifactAllTree {
		parentVisible := true
		if node.depth > 0 {
			parentVisible = visible[node.depth-1]
		}
		if node.isHeader {
			if !parentVisible {
				visible[node.depth] = false
				continue
			}
			if node.isSectionHeader {
				node.expanded = true
			} else if v, ok := m.artifactWorkspaceExpand[node.key]; ok {
				node.expanded = v
			}
			base = append(base, node)
			visible[node.depth] = node.expanded
			continue
		}
		if parentVisible {
			base = append(base, node)
		}
	}

	query := strings.ToLower(strings.TrimSpace(m.artifactSearchQuery))
	if query == "" {
		m.artifactTree = base
		return
	}

	// Search over the full indexed tree, independent of current expansion state.
	searchTree := make([]artifactTreeNode, 0, len(m.artifactAllTree))
	if scope := strings.TrimSpace(m.artifactSearchScopeKey); scope != "" {
		inScope := false
		scopeDepth := -1
		for _, n := range m.artifactAllTree {
			if n.key == scope {
				inScope = true
				scopeDepth = n.depth
				searchTree = append(searchTree, n)
				continue
			}
			if inScope && n.depth <= scopeDepth {
				inScope = false
			}
			if inScope {
				searchTree = append(searchTree, n)
			}
		}
	}
	if len(searchTree) == 0 {
		searchTree = m.artifactAllTree
	}

	include := map[int]struct{}{}
	ancestors := map[int]int{}
	matches := make([]int, 0, 32)
	for i, node := range searchTree {
		for d := range ancestors {
			if d > node.depth {
				delete(ancestors, d)
			}
		}
		if node.isHeader {
			ancestors[node.depth] = i
		}
		candidate := strings.ToLower(strings.TrimSpace(
			node.name + " " + node.vpath + " " + node.key + " " + node.role + " " + node.taskID + " " + node.goal + " " + node.status,
		))
		if strings.Contains(candidate, query) {
			matches = append(matches, i)
			include[i] = struct{}{}
			for _, ai := range ancestors {
				include[ai] = struct{}{}
			}
		}
	}
	for _, idx := range matches {
		node := searchTree[idx]
		if !node.isHeader {
			continue
		}
		for j := idx + 1; j < len(searchTree); j++ {
			if searchTree[j].depth <= node.depth {
				break
			}
			include[j] = struct{}{}
		}
	}
	if len(include) == 0 {
		m.artifactTree = nil
		return
	}
	filtered := make([]artifactTreeNode, 0, len(include))
	for i, node := range searchTree {
		if _, ok := include[i]; ok {
			if node.isHeader {
				node.expanded = true
			}
			filtered = append(filtered, node)
		}
	}
	m.artifactTree = filtered
}

func (m *monitorModel) currentArtifactAnchor() (string, int) {
	if m.artifactSelected >= 0 && m.artifactSelected < len(m.artifactTree) {
		return m.artifactTree[m.artifactSelected].key, m.artifactSelected
	}
	return "", 0
}

func (m *monitorModel) findArtifactNodeByKey(key string) int {
	key = strings.TrimSpace(key)
	if key == "" {
		return -1
	}
	for i, n := range m.artifactTree {
		if strings.TrimSpace(n.key) == key {
			return i
		}
	}
	return -1
}

func (m *monitorModel) handleArtifactTreeLoaded(msg artifactTreeLoadedMsg) tea.Cmd {
	if m == nil {
		return nil
	}
	if msg.err != nil {
		m.artifactContent = "Failed to load artifacts: " + msg.err.Error()
		m.artifactContentRaw = ""
		m.artifactRenderWidth = 0
		m.artifactRenderRawLen = 0
		m.artifactRenderedVPath = ""
		m.artifactTree = nil
		m.artifactContentVP.SetContent(m.artifactContent)
		m.artifactContentVP.GotoTop()
		return nil
	}

	m.artifactTasks = nil
	m.artifactWorkspaceFiles = nil
	m.artifactAllTree = m.buildArtifactTreeFromGroups(msg.nodes)
	m.applyArtifactVisibilityAndSearch()
	if len(m.artifactTree) == 0 {
		m.artifactSelected = 0
		m.artifactSelectedVPath = ""
		m.artifactContent = "No tasks or deliverables found. Completed tasks with artifacts will appear here."
		m.artifactContentRaw = ""
		m.artifactRenderWidth = 0
		m.artifactRenderRawLen = 0
		m.artifactRenderedVPath = ""
		m.artifactContentVP.SetContent(m.artifactContent)
		m.artifactContentVP.GotoTop()
		return nil
	}

	if strings.TrimSpace(m.artifactSelectedVPath) != "" {
		if idx := m.findArtifactNodeByKey("file:" + strings.TrimSpace(m.artifactSelectedVPath)); idx >= 0 {
			m.artifactSelected = idx
			node := m.artifactTree[idx]
			m.artifactContent = "Loading " + node.name + "..."
			m.artifactContentRaw = ""
			m.artifactRenderWidth = 0
			m.artifactRenderRawLen = 0
			m.artifactRenderedVPath = ""
			m.artifactContentVP.SetContent(m.artifactContent)
			m.artifactContentVP.GotoTop()
			return m.loadArtifactContent(node.vpath, node.diskPath, node.runID)
		}
	}

	for i, node := range m.artifactTree {
		if strings.HasPrefix(node.key, "task:") {
			m.artifactSelected = i
			return m.selectArtifactTreeNode()
		}
	}

	for i, node := range m.artifactTree {
		if strings.TrimSpace(node.vpath) == "" || node.isHeader || node.isSectionHeader || node.isWSHeader {
			continue
		}
		m.artifactSelected = i
		m.artifactSelectedVPath = node.vpath
		m.artifactContent = "Loading " + node.name + "..."
		m.artifactContentRaw = ""
		m.artifactRenderWidth = 0
		m.artifactRenderRawLen = 0
		m.artifactRenderedVPath = ""
		m.artifactContentVP.SetContent(m.artifactContent)
		m.artifactContentVP.GotoTop()
		return m.loadArtifactContent(node.vpath, node.diskPath, node.runID)
	}

	m.artifactSelected = 0
	m.artifactSelectedVPath = ""
	m.artifactContentRaw = ""
	m.artifactContent = "No tasks or deliverables found. Completed tasks with artifacts will appear here."
	m.artifactRenderWidth = 0
	m.artifactRenderRawLen = 0
	m.artifactRenderedVPath = ""
	m.artifactContentVP.SetContent(m.artifactContent)
	m.artifactContentVP.GotoTop()
	return nil
}

func (m *monitorModel) handleArtifactContentLoaded(msg artifactContentLoadedMsg) {
	if m == nil {
		return
	}
	if msg.err != nil {
		m.artifactContent = "Failed to load content: " + msg.err.Error()
		m.artifactContentRaw = ""
		m.artifactRenderWidth = 0
		m.artifactRenderRawLen = 0
		m.artifactRenderedVPath = ""
		m.artifactContentVP.SetContent(m.artifactContent)
		m.artifactContentVP.GotoTop()
		return
	}
	m.artifactSelectedVPath = strings.TrimSpace(msg.vpath)
	m.artifactContentRaw = msg.content
	m.artifactContent = ""
	m.artifactRenderWidth = 0
	m.artifactRenderRawLen = 0
	m.artifactRenderedVPath = ""
	m.refreshArtifactViewport()
}

func (m *monitorModel) refreshArtifactViewport() {
	if m == nil {
		return
	}
	frameW, frameH := m.styles.panel.GetFrameSize()
	_, contentW, bodyH := artifactLayout(m.width, m.height, frameW, frameH)
	contentInnerW := max(10, contentW)
	contentInnerH := max(1, bodyH-1)
	m.artifactContentVP.Width = contentInnerW
	m.artifactContentVP.Height = contentInnerH

	if strings.TrimSpace(m.artifactContentRaw) != "" {
		name := filepath.Base(strings.TrimSpace(m.artifactSelectedVPath))
		rawLen := len(m.artifactContentRaw)
		needsRender := m.artifactRenderWidth != contentInnerW ||
			m.artifactRenderedVPath != strings.TrimSpace(m.artifactSelectedVPath) ||
			m.artifactRenderRawLen != rawLen ||
			strings.TrimSpace(m.artifactContent) == ""
		if needsRender {
			rendered := renderArtifactByExt(name, m.artifactContentRaw, max(20, contentInnerW-2), m.renderer)
			rendered = strings.TrimRight(rendered, "\n")
			rendered = wrapViewportText(rendered, max(10, contentInnerW))
			m.artifactContent = rendered
			m.artifactRenderWidth = contentInnerW
			m.artifactRenderRawLen = rawLen
			m.artifactRenderedVPath = strings.TrimSpace(m.artifactSelectedVPath)
		}
	}
	if strings.TrimSpace(m.artifactContent) == "" {
		m.artifactContent = "Select a task or deliverable to preview its contents."
	}
	m.artifactContentVP.SetContent(m.artifactContent)
}

func (m *monitorModel) renderArtifactViewer() string {
	w := m.width
	if w <= 0 {
		w = 120
	}
	h := m.height
	if h <= 0 {
		h = 40
	}

	frameW, frameH := m.styles.panel.GetFrameSize()
	navW, contentW, bodyH := artifactLayout(w, h, frameW, frameH)
	m.refreshArtifactViewport()

	header := "Artifact Viewer"
	if strings.TrimSpace(m.artifactSelectedVPath) != "" {
		header += "  " + kit.StyleDim.Render(strings.TrimSpace(m.artifactSelectedVPath))
	}
	if q := strings.TrimSpace(m.artifactSearchQuery); q != "" {
		header += "  " + artifactStyleGroup.Render("[filter: "+q+"]")
	} else if m.artifactSearchMode {
		header += "  " + artifactStyleGroup.Render("[filter: typing...]")
	}
	headerLine := m.styles.header.Copy().MaxWidth(w).Render(header)

	navTitle := "Artifacts"
	if !m.artifactNavFocused {
		navTitle = kit.StyleDim.Render(navTitle)
	}
	contentTitle := "Preview"
	if m.artifactNavFocused {
		contentTitle = kit.StyleDim.Render(contentTitle)
	}

	navBody := m.renderArtifactNavBody(max(1, bodyH-1), max(10, navW))
	left := m.panelStyle(panelCurrentTask).Width(navW).Height(bodyH).Render(m.styles.sectionTitle.Render(navTitle) + "\n" + navBody)
	right := m.panelStyle(panelOutput).Width(contentW).Height(bodyH).Render(m.styles.sectionTitle.Render(contentTitle) + "\n" + m.artifactContentVP.View())
	row := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)

	footer := "esc close · j/k navigate · enter/l/right expand/select · h/left collapse · tab switch pane · pgup/pgdn scroll"
	if m.artifactSearchMode {
		footer = "search mode: type to filter (scoped to selected header) · enter done · esc clear/exit · backspace delete"
	} else {
		footer += " · / search"
	}
	footer = kit.TruncateRight(footer, max(1, w-2))
	footerLine := m.styles.header.Copy().MaxWidth(w).Render(kit.StyleDim.Render(footer))

	view := lipgloss.JoinVertical(lipgloss.Left, headerLine, row, footerLine)
	return lipgloss.NewStyle().MaxWidth(w).MaxHeight(h).Render(view)
}

func (m *monitorModel) renderArtifactNavBody(height, width int) string {
	if len(m.artifactTree) == 0 {
		if strings.TrimSpace(m.artifactSearchQuery) != "" {
			return kit.StyleDim.Render("No files match your filter.")
		}
		return kit.StyleDim.Render("No artifacts available.")
	}

	start := 0
	if m.artifactSelected >= height {
		start = m.artifactSelected - height + 1
	}
	if start < 0 {
		start = 0
	}
	if start > len(m.artifactTree)-1 {
		start = max(0, len(m.artifactTree)-1)
	}
	end := min(len(m.artifactTree), start+height)
	visible := make([]string, 0, max(0, end-start))
	for i := start; i < end; i++ {
		node := m.artifactTree[i]
		prefix := "  "
		if i == m.artifactSelected {
			prefix = "▸ "
		}
		nodeWidth := max(1, width-lipgloss.Width(prefix))
		line := kit.TruncateRight(prefix+m.renderArtifactNode(node, nodeWidth), max(1, width))
		if i == m.artifactSelected {
			line = m.styles.sectionTitle.Render(line)
		}
		visible = append(visible, line)
	}

	if len(visible) < height {
		pad := make([]string, 0, height-len(visible))
		for i := 0; i < height-len(visible); i++ {
			pad = append(pad, "")
		}
		visible = append(visible, pad...)
	}
	return strings.Join(visible, "\n")
}

func (m *monitorModel) renderArtifactNode(node artifactTreeNode, width int) string {
	indent := strings.Repeat("  ", max(0, node.depth))
	if node.isHeader {
		if node.isSectionHeader {
			label := strings.ToUpper(strings.TrimSpace(node.name))
			if label == "" {
				label = "SECTION"
			}
			return artifactStyleSection.Copy().Foreground(lipgloss.Color("#f38ba8")).Render(indent + "━━ " + label + " ━━")
		}
		icon := "▶"
		if node.expanded {
			icon = "▼"
		}
		if node.isDayHeader {
			return artifactStyleSection.Copy().Foreground(lipgloss.Color("#a6e3a1")).Render(indent + icon + " " + strings.TrimSpace(node.name))
		}
		if node.isRoleHeader {
			return artifactStyleGroup.Render(indent + icon + " " + strings.TrimSpace(node.name))
		}
		if node.isKindHeader {
			return artifactStyleSection.Copy().Foreground(lipgloss.Color("#f9e2af")).Render(indent + icon + " " + strings.TrimSpace(node.name))
		}
		if node.isTaskHeader {
			goal := strings.TrimSpace(node.goal)
			if goal == "" {
				goal = node.taskID
			}
			base := indent + icon + " "
			suffix := " " + taskStatusMark(node.status)
			goalMax := max(1, width-lipgloss.Width(base)-lipgloss.Width(suffix))
			return base + truncateText(goal, goalMax) + suffix
		}
		label := strings.TrimSpace(node.name)
		return artifactStyleGroup.Render(indent + icon + " " + label)
	}
	if strings.HasPrefix(node.key, "task:") {
		goal := strings.TrimSpace(node.goal)
		if goal == "" {
			goal = strings.TrimSpace(node.name)
		}
		if goal == "" {
			goal = node.taskID
		}
		base := indent + "• "
		suffix := " " + taskStatusMark(node.status)
		goalMax := max(1, width-lipgloss.Width(base)-lipgloss.Width(suffix))
		return artifactStyleFile.Render(base + truncateText(goal, goalMax) + suffix)
	}
	name := node.name
	if node.isSummary {
		name = "SUMMARY.md"
	}
	if node.isUnreported {
		name += " (unreported)"
	}
	return artifactStyleFile.Render(indent + "• " + name)
}

func artifactLayout(width, height, frameW, frameH int) (navW, contentW, bodyH int) {
	if width <= 0 {
		width = 120
	}
	if height <= 0 {
		height = 40
	}
	const (
		gapCols    = 1
		navMin     = 16
		contentMin = 20
	)
	usableW := width - (2 * frameW) - gapCols
	if usableW < 2 {
		usableW = 2
	}

	if usableW >= navMin+contentMin {
		navW = usableW / 3
		if navW < navMin {
			navW = navMin
		}
		maxNav := usableW - contentMin
		if navW > maxNav {
			navW = maxNav
		}
		contentW = usableW - navW
	} else {
		navW = usableW / 3
		if navW < 1 {
			navW = 1
		}
		contentW = usableW - navW
		if contentW < 1 {
			contentW = 1
			navW = usableW - contentW
		}
	}
	bodyH = max(1, height-2-frameH)
	return navW, contentW, bodyH
}

func viewportWithMouseDisabled(vp viewport.Model) viewport.Model {
	if vp.Width == 0 && vp.Height == 0 {
		vp = viewport.New(0, 0)
	}
	vp.MouseWheelEnabled = false
	return vp
}
