package tui

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	agentstate "github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
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
	status string
	runID  string

	vpath     string
	name      string
	isSummary bool

	isHeader         bool
	isRoleHeader     bool
	isTaskHeader     bool
	isWorkspaceGroup bool
	isWSHeader       bool
	expanded         bool
	depth            int
}

type artifactTreeLoadedMsg struct {
	tasks   []types.Task
	wsFiles []artifactTreeNode
	err     error
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
	m.artifactWorkspaceFiles = nil
	m.artifactSelected = 0
	m.artifactSelectedVPath = ""
	m.artifactContentRaw = ""
	m.artifactContent = "Loading artifacts..."
	m.artifactRenderWidth = 0
	m.artifactRenderRawLen = 0
	m.artifactRenderedVPath = ""
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
	m.artifactWorkspaceFiles = nil
	m.artifactSelected = 0
	m.artifactSelectedVPath = ""
	m.artifactContent = ""
	m.artifactContentRaw = ""
	m.artifactRenderWidth = 0
	m.artifactRenderRawLen = 0
	m.artifactRenderedVPath = ""
	m.artifactNavFocused = false
	m.artifactRoleExpanded = nil
	m.artifactTaskExpanded = nil
	m.artifactWorkspaceExpand = nil
}

func (m *monitorModel) loadArtifactTree() tea.Cmd {
	if m == nil || m.taskStore == nil {
		return func() tea.Msg {
			return artifactTreeLoadedMsg{err: fmt.Errorf("task store is not available")}
		}
	}
	taskStore := m.taskStore
	ctx := m.ctx
	teamID := strings.TrimSpace(m.teamID)
	runID := strings.TrimSpace(m.runID)
	dataDir := strings.TrimSpace(m.cfg.DataDir)
	teamRunIDs := append([]string(nil), m.teamRunIDs...)
	roleByRunID := map[string]string{}
	for k, v := range m.teamRoleByRunID {
		roleByRunID[k] = v
	}

	return func() tea.Msg {
		filter := agentstate.TaskFilter{
			Status:   []types.TaskStatus{types.TaskStatusSucceeded, types.TaskStatusFailed},
			SortBy:   "finished_at",
			SortDesc: true,
			Limit:    200,
		}
		if teamID != "" {
			filter.TeamID = teamID
		} else {
			filter.RunID = runID
		}

		tasks, err := taskStore.ListTasks(ctx, filter)
		if err != nil {
			return artifactTreeLoadedMsg{err: err}
		}

		tasksForRoleMap := make([]types.Task, 0, len(tasks))
		for _, t := range tasks {
			taskID := strings.TrimSpace(t.TaskID)
			if taskID == "" {
				continue
			}
			tasksForRoleMap = append(tasksForRoleMap, t)
		}

		wsFiles := scanArtifactWorkspaceFiles(dataDir, teamID, runID, teamRunIDs, tasksForRoleMap, roleByRunID)
		return artifactTreeLoadedMsg{tasks: tasksForRoleMap, wsFiles: wsFiles}
	}
}

func dedupeArtifactPaths(paths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if !strings.HasPrefix(p, "/workspace/") {
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
		nodes = append(nodes, artifactTreeNode{
			key:              "wsgroup:" + rid,
			runID:            rid,
			name:             rid,
			isHeader:         true,
			isWorkspaceGroup: true,
			expanded:         false,
			depth:            1,
		})
		for _, rel := range files {
			vpath := "/workspace/" + rel
			nodes = append(nodes, artifactTreeNode{
				key:   "wsfile:" + vpath,
				runID: rid,
				vpath: vpath,
				name:  rel,
				depth: 2,
			})
		}
	}
	return nodes
}

func scanTeamWorkspaceFiles(dataDir, teamID string, tasks []types.Task, roleByRunID map[string]string) []artifactTreeNode {
	wsDir := fsutil.GetTeamWorkspaceDir(dataDir, teamID)
	relFiles := scanWorkspaceRelativeFiles(wsDir, 50000)
	if len(relFiles) == 0 {
		return nil
	}

	roleByTaskID := map[string]string{}
	for _, t := range tasks {
		tid := strings.TrimSpace(t.TaskID)
		if tid == "" {
			continue
		}
		roleByTaskID[tid] = roleForTask(t, roleByRunID)
	}

	grouped := map[string][]artifactTreeNode{}
	for _, rel := range relFiles {
		role := inferWorkspaceRole(rel, roleByTaskID)
		vpath := "/workspace/" + rel
		grouped[role] = append(grouped[role], artifactTreeNode{
			key:   "wsfile:" + vpath,
			vpath: vpath,
			name:  rel,
			role:  role,
			depth: 2,
		})
	}

	roles := make([]string, 0, len(grouped))
	for role := range grouped {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	nodes := make([]artifactTreeNode, 0, len(relFiles)+len(roles))
	for _, role := range roles {
		nodes = append(nodes, artifactTreeNode{
			key:              "wsgroup:" + role,
			role:             role,
			name:             role,
			isHeader:         true,
			isWorkspaceGroup: true,
			expanded:         false,
			depth:            1,
		})
		files := grouped[role]
		sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
		nodes = append(nodes, files...)
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

func inferWorkspaceRole(rel string, roleByTaskID map[string]string) string {
	rel = strings.TrimSpace(filepath.ToSlash(rel))
	if rel == "" {
		return artifactWorkspaceSharedGroup
	}
	parts := strings.Split(rel, "/")
	if len(parts) >= 3 && parts[0] == "deliverables" {
		taskID := strings.TrimSpace(parts[2])
		if taskID != "" {
			if role := strings.TrimSpace(roleByTaskID[taskID]); role != "" {
				return role
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

func (m *monitorModel) buildArtifactTree(tasks []types.Task, wsFiles []artifactTreeNode) []artifactTreeNode {
	_ = tasks
	return appendWorkspaceSection(nil, wsFiles, m.artifactWorkspaceExpand)
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
	return appendWorkspaceSection(tree, wsFiles, m.artifactWorkspaceExpand)
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

	return appendWorkspaceSection(tree, wsFiles, m.artifactWorkspaceExpand)
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

func appendWorkspaceSection(tree, wsFiles []artifactTreeNode, expand map[string]bool) []artifactTreeNode {
	if len(wsFiles) == 0 {
		return tree
	}
	tree = append(tree, artifactTreeNode{
		key:        "ws:section",
		name:       "All Workspace Files",
		isWSHeader: true,
		depth:      0,
	})
	showChildren := false
	for _, n := range wsFiles {
		node := n
		if node.isWorkspaceGroup {
			expanded := node.expanded
			if expand != nil {
				if v, ok := expand[node.key]; ok {
					expanded = v
				} else {
					expand[node.key] = expanded
				}
			}
			node.expanded = expanded
			tree = append(tree, node)
			showChildren = expanded
			continue
		}
		if showChildren {
			tree = append(tree, node)
		}
	}
	return tree
}

func (m *monitorModel) loadArtifactContent(vpath, runID string) tea.Cmd {
	vpath = strings.TrimSpace(vpath)
	if vpath == "" {
		return nil
	}
	dataDir := strings.TrimSpace(m.cfg.DataDir)
	teamID := strings.TrimSpace(m.teamID)
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = strings.TrimSpace(m.runID)
	}

	return func() tea.Msg {
		diskPath := resolveArtifactDisk(dataDir, teamID, runID, vpath)
		if diskPath == "" {
			return artifactContentLoadedMsg{vpath: vpath, err: fmt.Errorf("unsupported path: %s", vpath)}
		}
		f, err := os.Open(diskPath)
		if err != nil {
			return artifactContentLoadedMsg{vpath: vpath, err: err}
		}
		defer f.Close()

		buf := make([]byte, artifactFileReadLimitBytes+1)
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			return artifactContentLoadedMsg{vpath: vpath, err: err}
		}
		if n > artifactFileReadLimitBytes {
			n = artifactFileReadLimitBytes
		}
		content := string(buf[:n])
		if n == artifactFileReadLimitBytes {
			content += "\n\n[truncated: file exceeds 256KB]"
		}
		return artifactContentLoadedMsg{vpath: vpath, content: content}
	}
}

func resolveArtifactDisk(dataDir, teamID, runID, vpath string) string {
	vpath = strings.TrimSpace(vpath)
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
		m.closeArtifactViewer()
		return m, nil
	case "tab":
		m.artifactNavFocused = !m.artifactNavFocused
		return m, nil
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
	case "j", "down":
		if m.artifactSelected < len(m.artifactTree)-1 {
			m.artifactSelected++
		}
		return m, nil
	case "k", "up":
		if m.artifactSelected > 0 {
			m.artifactSelected--
		}
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

func (m *monitorModel) selectArtifactTreeNode() tea.Cmd {
	if m == nil || m.artifactSelected < 0 || m.artifactSelected >= len(m.artifactTree) {
		return nil
	}
	node := m.artifactTree[m.artifactSelected]
	if node.isWSHeader {
		return nil
	}
	if node.isHeader {
		if m.artifactRoleExpanded == nil {
			m.artifactRoleExpanded = map[string]bool{}
		}
		if m.artifactTaskExpanded == nil {
			m.artifactTaskExpanded = map[string]bool{}
		}
		if m.artifactWorkspaceExpand == nil {
			m.artifactWorkspaceExpand = map[string]bool{}
		}
		if node.isRoleHeader {
			m.artifactRoleExpanded[node.role] = !node.expanded
		} else if node.isTaskHeader {
			m.artifactTaskExpanded[node.taskID] = !node.expanded
		} else if node.isWorkspaceGroup {
			m.artifactWorkspaceExpand[node.key] = !node.expanded
		}
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
	return m.loadArtifactContent(node.vpath, node.runID)
}

func (m *monitorModel) collapseSelectedArtifactGroup() {
	if m == nil || m.artifactSelected < 0 || m.artifactSelected >= len(m.artifactTree) {
		return
	}
	if m.artifactRoleExpanded == nil {
		m.artifactRoleExpanded = map[string]bool{}
	}
	if m.artifactTaskExpanded == nil {
		m.artifactTaskExpanded = map[string]bool{}
	}
	if m.artifactWorkspaceExpand == nil {
		m.artifactWorkspaceExpand = map[string]bool{}
	}

	node := m.artifactTree[m.artifactSelected]
	if node.isHeader {
		if node.isRoleHeader && node.expanded {
			m.artifactRoleExpanded[node.role] = false
			m.rebuildArtifactTreeWithAnchor(node.key, m.artifactSelected)
			return
		}
		if node.isTaskHeader && node.expanded {
			m.artifactTaskExpanded[node.taskID] = false
			m.rebuildArtifactTreeWithAnchor(node.key, m.artifactSelected)
			return
		}
		if node.isWorkspaceGroup && node.expanded {
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
		if parent.isTaskHeader && parent.expanded {
			m.artifactTaskExpanded[parent.taskID] = false
			m.rebuildArtifactTreeWithAnchor(parent.key, i)
			return
		}
		if parent.isRoleHeader && parent.expanded {
			m.artifactRoleExpanded[parent.role] = false
			m.rebuildArtifactTreeWithAnchor(parent.key, i)
			return
		}
		if parent.isWorkspaceGroup && parent.expanded {
			m.artifactWorkspaceExpand[parent.key] = false
			m.rebuildArtifactTreeWithAnchor(parent.key, i)
			return
		}
		break
	}
}

func (m *monitorModel) rebuildArtifactTreeWithAnchor(anchorKey string, anchorIndex int) {
	m.artifactTree = m.buildArtifactTree(m.artifactTasks, m.artifactWorkspaceFiles)
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

	m.artifactTasks = msg.tasks
	m.artifactWorkspaceFiles = msg.wsFiles
	m.artifactTree = m.buildArtifactTree(msg.tasks, msg.wsFiles)
	if len(m.artifactTree) == 0 {
		m.artifactSelected = 0
		m.artifactSelectedVPath = ""
		m.artifactContent = "No deliverables found. Completed tasks with artifacts will appear here."
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
			return m.loadArtifactContent(node.vpath, node.runID)
		}
	}

	for i, node := range m.artifactTree {
		if strings.TrimSpace(node.vpath) == "" || node.isHeader || node.isWSHeader {
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
		return m.loadArtifactContent(node.vpath, node.runID)
	}

	m.artifactSelected = 0
	m.artifactSelectedVPath = ""
	m.artifactContentRaw = ""
	m.artifactContent = "Select a workspace group and file to preview."
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
	_, contentW, bodyH := artifactLayout(m.width, m.height)
	contentInnerW := max(10, contentW-2)
	contentInnerH := max(1, bodyH)
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
		m.artifactContent = "Select a deliverable file to preview its contents."
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

	navW, contentW, bodyH := artifactLayout(w, h)
	m.refreshArtifactViewport()

	header := "Artifact Viewer"
	if strings.TrimSpace(m.artifactSelectedVPath) != "" {
		header += "  " + kit.StyleDim.Render(strings.TrimSpace(m.artifactSelectedVPath))
	}
	headerLine := m.styles.header.Copy().MaxWidth(w).Render(header)

	navTitle := "Deliverables"
	if !m.artifactNavFocused {
		navTitle = kit.StyleDim.Render(navTitle)
	}
	contentTitle := "Preview"
	if m.artifactNavFocused {
		contentTitle = kit.StyleDim.Render(contentTitle)
	}

	navBody := m.renderArtifactNavBody(max(1, bodyH-1), max(10, navW-2))
	left := m.panelStyle(panelCurrentTask).Width(navW).Height(bodyH).Render(m.styles.sectionTitle.Render(navTitle) + "\n" + navBody)
	right := m.panelStyle(panelOutput).Width(contentW).Height(bodyH).Render(m.styles.sectionTitle.Render(contentTitle) + "\n" + m.artifactContentVP.View())
	row := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	footer := "esc close · j/k navigate · enter/l/right select or toggle · h/left collapse · tab switch pane · pgup/pgdn scroll"
	footer = kit.TruncateRight(footer, max(1, w-2))
	footerLine := m.styles.header.Copy().MaxWidth(w).Render(kit.StyleDim.Render(footer))

	view := lipgloss.JoinVertical(lipgloss.Left, headerLine, row, footerLine)
	return lipgloss.NewStyle().MaxWidth(w).MaxHeight(h).Render(view)
}

func (m *monitorModel) renderArtifactNavBody(height, width int) string {
	if len(m.artifactTree) == 0 {
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
		line := kit.TruncateRight(prefix+m.renderArtifactNode(node), max(1, width))
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

func (m *monitorModel) renderArtifactNode(node artifactTreeNode) string {
	indent := strings.Repeat("  ", max(0, node.depth))
	if node.isWSHeader {
		return artifactStyleSection.Render(indent + "-- " + node.name + " --")
	}
	if node.isHeader {
		icon := "▶"
		if node.expanded {
			icon = "▼"
		}
		if node.isRoleHeader {
			return indent + icon + " " + strings.TrimSpace(node.role)
		}
		if node.isWorkspaceGroup {
			label := strings.TrimSpace(node.name)
			if label == "" {
				label = artifactWorkspaceSharedGroup
			}
			return artifactStyleGroup.Render(indent + icon + " " + label)
		}
		if node.isTaskHeader {
			status := strings.TrimSpace(node.status)
			statusMark := ""
			switch status {
			case string(types.TaskStatusSucceeded):
				statusMark = " ✓"
			case string(types.TaskStatusFailed):
				statusMark = " ✗"
			default:
				if status != "" {
					statusMark = " [" + status + "]"
				}
			}
			goal := strings.TrimSpace(node.goal)
			if goal == "" {
				goal = node.taskID
			}
			return indent + icon + " " + truncateText(goal, 44) + statusMark
		}
		label := strings.TrimSpace(node.name)
		return artifactStyleGroup.Render(indent + icon + " " + label)
	}
	name := node.name
	if node.isSummary {
		name = "SUMMARY.md"
	}
	return artifactStyleFile.Render(indent + "• " + name)
}

func artifactLayout(width, height int) (navW, contentW, bodyH int) {
	if width <= 0 {
		width = 120
	}
	if height <= 0 {
		height = 40
	}
	navW = max(30, width/3)
	if navW > width-28 {
		navW = max(24, width-28)
	}
	contentW = max(24, width-navW)
	bodyH = max(8, height-4)
	return navW, contentW, bodyH
}

func viewportWithMouseDisabled(vp viewport.Model) viewport.Model {
	if vp.Width == 0 && vp.Height == 0 {
		vp = viewport.New(0, 0)
	}
	vp.MouseWheelEnabled = false
	return vp
}
