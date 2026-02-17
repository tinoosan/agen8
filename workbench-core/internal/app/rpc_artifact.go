package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/protocol"
)

const (
	artifactListDefaultLimit = 200
	artifactListMaxLimit     = 2000
	artifactSearchDefault    = 100
	artifactSearchMax        = 1000
	artifactGetDefaultBytes  = 256 * 1024
	artifactGetMaxBytes      = 1024 * 1024
)

func registerArtifactHandlers(s *RPCServer, reg methodRegistry) error {
	return registerHandlers(
		func() error {
			return addBoundHandler[protocol.ArtifactListParams, protocol.ArtifactListResult](reg, protocol.MethodArtifactList, false, s.artifactList)
		},
		func() error {
			return addBoundHandler[protocol.ArtifactSearchParams, protocol.ArtifactSearchResult](reg, protocol.MethodArtifactSearch, false, s.artifactSearch)
		},
		func() error {
			return addBoundHandler[protocol.ArtifactGetParams, protocol.ArtifactGetResult](reg, protocol.MethodArtifactGet, false, s.artifactGet)
		},
	)
}

type artifactScope struct {
	sessionID string
	teamID    string
	runID     string
}

func standaloneVisibleArtifactVPath(vpath string) bool {
	vpath = strings.TrimSpace(vpath)
	if strings.HasPrefix(vpath, "/tasks/") {
		rel := strings.TrimSpace(strings.TrimPrefix(vpath, "/tasks/"))
		return rel != ""
	}
	if strings.HasPrefix(vpath, "/deliverables/") {
		rel := strings.TrimSpace(strings.TrimPrefix(vpath, "/deliverables/"))
		return rel != ""
	}
	if !strings.HasPrefix(vpath, "/workspace/") {
		return false
	}
	rel := strings.TrimSpace(strings.TrimPrefix(vpath, "/workspace/"))
	return rel != ""
}

func filterStandaloneArtifactGroups(groups []state.ArtifactGroup) []state.ArtifactGroup {
	if len(groups) == 0 {
		return groups
	}
	out := make([]state.ArtifactGroup, 0, len(groups))
	for _, g := range groups {
		filtered := make([]state.ArtifactRecord, 0, len(g.Files))
		for _, f := range g.Files {
			if standaloneVisibleArtifactVPath(f.VPath) {
				filtered = append(filtered, f)
			}
		}
		if len(filtered) == 0 {
			continue
		}
		g.Files = filtered
		out = append(out, g)
	}
	return out
}

func filterStandaloneArtifactRecords(records []state.ArtifactRecord) []state.ArtifactRecord {
	if len(records) == 0 {
		return records
	}
	out := make([]state.ArtifactRecord, 0, len(records))
	for _, r := range records {
		if standaloneVisibleArtifactVPath(r.VPath) {
			out = append(out, r)
		}
	}
	return out
}

func (s *RPCServer) sessionRunIDs(ctx context.Context, sessionID string, fallbackRunID string) []string {
	sessionID = strings.TrimSpace(sessionID)
	fallbackRunID = strings.TrimSpace(fallbackRunID)
	out := []string{}
	seen := map[string]struct{}{}
	add := func(runID string) {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			return
		}
		if _, ok := seen[runID]; ok {
			return
		}
		seen[runID] = struct{}{}
		out = append(out, runID)
	}
	if sessionID == "" || s == nil || s.taskService == nil {
		add(fallbackRunID)
		return out
	}
	if sess, err := s.loadSessionForID(ctx, sessionID); err == nil {
		add(sess.CurrentRunID)
		for _, runID := range sess.Runs {
			add(runID)
		}
	}
	const pageSize = 500
	for offset := 0; ; offset += pageSize {
		tasks, err := s.taskService.ListTasks(ctx, state.TaskFilter{
			SessionID: sessionID,
			SortBy:    "created_at",
			SortDesc:  true,
			Limit:     pageSize,
			Offset:    offset,
		})
		if err != nil || len(tasks) == 0 {
			break
		}
		for _, t := range tasks {
			add(t.RunID)
		}
		if len(tasks) < pageSize {
			break
		}
	}
	add(fallbackRunID)
	sort.Strings(out)
	return out
}

func mergeArtifactGroups(groups []state.ArtifactGroup) []state.ArtifactGroup {
	if len(groups) <= 1 {
		return groups
	}
	mergedByTask := make(map[string]state.ArtifactGroup, len(groups))
	order := make([]string, 0, len(groups))
	for _, g := range groups {
		taskID := strings.TrimSpace(g.TaskID)
		if taskID == "" {
			continue
		}
		existing, ok := mergedByTask[taskID]
		if !ok {
			cp := g
			cp.Files = append([]state.ArtifactRecord(nil), g.Files...)
			mergedByTask[taskID] = cp
			order = append(order, taskID)
			continue
		}
		seenArtifact := map[string]struct{}{}
		for _, f := range existing.Files {
			key := strings.TrimSpace(f.ArtifactID)
			if key == "" {
				key = strings.TrimSpace(f.VPath)
			}
			seenArtifact[key] = struct{}{}
		}
		for _, f := range g.Files {
			key := strings.TrimSpace(f.ArtifactID)
			if key == "" {
				key = strings.TrimSpace(f.VPath)
			}
			if _, exists := seenArtifact[key]; exists {
				continue
			}
			seenArtifact[key] = struct{}{}
			existing.Files = append(existing.Files, f)
		}
		if g.ProducedAt.After(existing.ProducedAt) {
			existing.ProducedAt = g.ProducedAt
		}
		mergedByTask[taskID] = existing
	}
	out := make([]state.ArtifactGroup, 0, len(order))
	for _, taskID := range order {
		if g, ok := mergedByTask[taskID]; ok {
			sort.SliceStable(g.Files, func(i, j int) bool {
				if g.Files[i].IsSummary != g.Files[j].IsSummary {
					return g.Files[i].IsSummary
				}
				return strings.TrimSpace(g.Files[i].DisplayName) < strings.TrimSpace(g.Files[j].DisplayName)
			})
			out = append(out, g)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DayBucket != out[j].DayBucket {
			return out[i].DayBucket > out[j].DayBucket
		}
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		if !out[i].ProducedAt.Equal(out[j].ProducedAt) {
			return out[i].ProducedAt.After(out[j].ProducedAt)
		}
		return out[i].TaskID < out[j].TaskID
	})
	return out
}

func mergeArtifactRecords(records []state.ArtifactRecord) []state.ArtifactRecord {
	if len(records) <= 1 {
		return records
	}
	out := make([]state.ArtifactRecord, 0, len(records))
	seen := map[string]struct{}{}
	for _, r := range records {
		key := strings.TrimSpace(r.ArtifactID)
		if key == "" {
			key = strings.TrimSpace(r.VPath)
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].ProducedAt.Equal(out[j].ProducedAt) {
			return out[i].ProducedAt.After(out[j].ProducedAt)
		}
		if out[i].TaskID != out[j].TaskID {
			return out[i].TaskID < out[j].TaskID
		}
		return strings.TrimSpace(out[i].DisplayName) < strings.TrimSpace(out[j].DisplayName)
	})
	return out
}

func (s *RPCServer) listArtifactGroupsForScope(ctx context.Context, indexer state.ArtifactIndexer, scope artifactScope, filter state.ArtifactFilter) ([]state.ArtifactGroup, error) {
	if strings.TrimSpace(scope.teamID) != "" {
		filter.TeamID = strings.TrimSpace(scope.teamID)
		filter.RunID = ""
		return indexer.ListArtifactGroups(ctx, filter)
	}
	runIDs := s.sessionRunIDs(ctx, scope.sessionID, scope.runID)
	if len(runIDs) <= 1 {
		filter.TeamID = ""
		filter.RunID = strings.TrimSpace(scope.runID)
		groups, err := indexer.ListArtifactGroups(ctx, filter)
		if err != nil {
			return nil, err
		}
		return filterStandaloneArtifactGroups(groups), nil
	}
	all := make([]state.ArtifactGroup, 0, len(runIDs)*8)
	for _, runID := range runIDs {
		runFilter := filter
		runFilter.TeamID = ""
		runFilter.RunID = runID
		groups, err := indexer.ListArtifactGroups(ctx, runFilter)
		if err != nil {
			return nil, err
		}
		all = append(all, groups...)
	}
	return filterStandaloneArtifactGroups(mergeArtifactGroups(all)), nil
}

func (s *RPCServer) searchArtifactsForScope(ctx context.Context, indexer state.ArtifactIndexer, scope artifactScope, filter state.ArtifactSearchFilter) ([]state.ArtifactRecord, error) {
	if strings.TrimSpace(scope.teamID) != "" {
		filter.TeamID = strings.TrimSpace(scope.teamID)
		filter.RunID = ""
		return indexer.SearchArtifacts(ctx, filter)
	}
	runIDs := s.sessionRunIDs(ctx, scope.sessionID, scope.runID)
	if len(runIDs) <= 1 {
		filter.TeamID = ""
		filter.RunID = strings.TrimSpace(scope.runID)
		rows, err := indexer.SearchArtifacts(ctx, filter)
		if err != nil {
			return nil, err
		}
		return filterStandaloneArtifactRecords(rows), nil
	}
	all := make([]state.ArtifactRecord, 0, len(runIDs)*8)
	for _, runID := range runIDs {
		runFilter := filter
		runFilter.TeamID = ""
		runFilter.RunID = runID
		rows, err := indexer.SearchArtifacts(ctx, runFilter)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
	}
	return filterStandaloneArtifactRecords(mergeArtifactRecords(all)), nil
}

func (s *RPCServer) resolveArtifactScope(ctx context.Context, threadID protocol.ThreadID, teamIDOverride string) (artifactScope, error) {
	resolvedThread, err := s.resolveThreadID(threadID)
	if err != nil {
		return artifactScope{}, err
	}

	teamID := strings.TrimSpace(teamIDOverride)
	scope := artifactScope{sessionID: resolvedThread}
	if sess, serr := s.loadSessionForID(ctx, resolvedThread); serr == nil {
		scope.sessionID = strings.TrimSpace(sess.SessionID)
		scope.runID = defaultRunIDForSession(sess)
		if teamID == "" {
			teamID = strings.TrimSpace(sess.TeamID)
		}
	}
	if teamID == "" && s.taskService != nil && strings.TrimSpace(scope.runID) != "" {
		tasks, err := s.taskService.ListTasks(ctx, state.TaskFilter{
			RunID:    strings.TrimSpace(scope.runID),
			SortBy:   "created_at",
			SortDesc: true,
			Limit:    1,
		})
		if err == nil && len(tasks) != 0 {
			teamID = strings.TrimSpace(tasks[0].TeamID)
		}
	}
	if teamID != "" {
		scope.teamID = teamID
		return scope, nil
	}
	if strings.TrimSpace(scope.runID) == "" {
		return artifactScope{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "run scope is unavailable for thread"}
	}
	return scope, nil
}

func (s *RPCServer) artifactIndexer() (state.ArtifactIndexer, error) {
	if s == nil || s.taskService == nil {
		return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "task store not configured"}
	}
	indexer, ok := s.taskService.ArtifactIndexer()
	if !ok {
		return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "artifact index is unavailable"}
	}
	return indexer, nil
}

func (s *RPCServer) artifactList(ctx context.Context, p protocol.ArtifactListParams) (protocol.ArtifactListResult, error) {
	scope, err := s.resolveArtifactScope(ctx, p.ThreadID, p.TeamID)
	if err != nil {
		return protocol.ArtifactListResult{}, err
	}
	indexer, err := s.artifactIndexer()
	if err != nil {
		return protocol.ArtifactListResult{}, err
	}
	groups, err := s.listArtifactGroupsForScope(ctx, indexer, scope, state.ArtifactFilter{
		DayBucket: strings.TrimSpace(p.DayBucket),
		Role:      strings.TrimSpace(p.Role),
		TaskKind:  strings.TrimSpace(p.TaskKind),
		TaskID:    strings.TrimSpace(p.TaskID),
		Limit:     clampLimit(p.Limit, artifactListDefaultLimit, artifactListMaxLimit),
	})
	if err != nil {
		return protocol.ArtifactListResult{}, err
	}
	return protocol.ArtifactListResult{Nodes: artifactGroupsToNodes(groups)}, nil
}

func applyScopeKey(filter *state.ArtifactFilter, scopeKey string) {
	if filter == nil {
		return
	}
	scopeKey = strings.TrimSpace(scopeKey)
	if scopeKey == "" {
		return
	}
	switch {
	case strings.HasPrefix(scopeKey, "day:"):
		if filter.DayBucket == "" {
			filter.DayBucket = strings.TrimPrefix(scopeKey, "day:")
		}
	case strings.HasPrefix(scopeKey, "role:"):
		parts := strings.Split(scopeKey, ":")
		if len(parts) >= 3 {
			if filter.DayBucket == "" {
				filter.DayBucket = strings.TrimSpace(parts[1])
			}
			if filter.Role == "" {
				filter.Role = strings.TrimSpace(parts[2])
			}
		}
	case strings.HasPrefix(scopeKey, "kind:"):
		parts := strings.Split(scopeKey, ":")
		if len(parts) >= 4 {
			if filter.DayBucket == "" {
				filter.DayBucket = strings.TrimSpace(parts[1])
			}
			if filter.Role == "" {
				filter.Role = strings.TrimSpace(parts[2])
			}
			if filter.TaskKind == "" {
				filter.TaskKind = strings.TrimSpace(parts[3])
			}
		}
	case strings.HasPrefix(scopeKey, "task:"):
		if filter.TaskID == "" {
			filter.TaskID = strings.TrimPrefix(scopeKey, "task:")
		}
	}
}

func (s *RPCServer) artifactSearch(ctx context.Context, p protocol.ArtifactSearchParams) (protocol.ArtifactSearchResult, error) {
	query := strings.TrimSpace(p.Query)
	if query == "" {
		return protocol.ArtifactSearchResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "query is required"}
	}
	scope, err := s.resolveArtifactScope(ctx, p.ThreadID, p.TeamID)
	if err != nil {
		return protocol.ArtifactSearchResult{}, err
	}
	indexer, err := s.artifactIndexer()
	if err != nil {
		return protocol.ArtifactSearchResult{}, err
	}
	filter := state.ArtifactFilter{
		TeamID:    strings.TrimSpace(scope.teamID),
		RunID:     strings.TrimSpace(scope.runID),
		DayBucket: strings.TrimSpace(p.DayBucket),
		Role:      strings.TrimSpace(p.Role),
		TaskKind:  strings.TrimSpace(p.TaskKind),
		TaskID:    strings.TrimSpace(p.TaskID),
		Limit:     clampLimit(p.Limit, artifactSearchDefault, artifactSearchMax),
	}
	applyScopeKey(&filter, p.ScopeKey)
	matches, err := s.searchArtifactsForScope(ctx, indexer, scope, state.ArtifactSearchFilter{
		ArtifactFilter: filter,
		Query:          query,
	})
	if err != nil {
		return protocol.ArtifactSearchResult{}, err
	}
	if len(matches) == 0 {
		return protocol.ArtifactSearchResult{Nodes: nil, MatchCount: 0}, nil
	}
	groups, err := s.listArtifactGroupsForScope(ctx, indexer, scope, state.ArtifactFilter{
		DayBucket: filter.DayBucket,
		Role:      filter.Role,
		TaskKind:  filter.TaskKind,
		TaskID:    filter.TaskID,
		Limit:     artifactListMaxLimit,
	})
	if err != nil {
		return protocol.ArtifactSearchResult{}, err
	}
	byTask := make(map[string]state.ArtifactGroup, len(groups))
	for _, g := range groups {
		byTask[strings.TrimSpace(g.TaskID)] = g
	}
	nodes := searchMatchesToNodes(matches, byTask)
	return protocol.ArtifactSearchResult{
		Nodes:      nodes,
		MatchCount: len(matches),
	}, nil
}

func resolveArtifactDiskPath(dataDir, teamID, runID, vpath string) string {
	vpath = strings.TrimSpace(vpath)
	if vpath == "" {
		return ""
	}
	if strings.HasPrefix(vpath, "/tasks/") {
		rel := strings.TrimPrefix(vpath, "/tasks/")
		rel = strings.TrimPrefix(rel, "/")
		if strings.TrimSpace(teamID) != "" {
			base := fsutil.GetTeamTasksDir(dataDir, teamID)
			candidate := filepath.Join(base, rel)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			return candidate
		}
		base := fsutil.GetTasksDir(dataDir, runID)
		candidate := filepath.Join(base, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		return candidate
	}
	if strings.HasPrefix(vpath, "/deliverables/") {
		rel := strings.TrimPrefix(vpath, "/deliverables/")
		rel = strings.TrimPrefix(rel, "/")
		base := fsutil.GetDeliverablesDir(dataDir, runID)
		candidate := filepath.Join(base, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		return candidate
	}
	if !strings.HasPrefix(vpath, "/workspace/") {
		return ""
	}
	rel := strings.TrimPrefix(vpath, "/workspace/")
	if strings.TrimSpace(teamID) != "" {
		base := fsutil.GetTeamWorkspaceDir(dataDir, teamID)
		candidate := filepath.Join(base, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		return candidate
	}
	base := fsutil.GetWorkspaceDir(dataDir, runID)
	candidate := filepath.Join(base, rel)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return candidate
}

func (s *RPCServer) resolveArtifactDiskPathForScope(ctx context.Context, scope artifactScope, preferredTeamID, preferredRunID, vpath string) string {
	vpath = strings.TrimSpace(vpath)
	if vpath == "" {
		return ""
	}
	if teamID := strings.TrimSpace(preferredTeamID); teamID != "" {
		return resolveArtifactDiskPath(s.cfg.DataDir, teamID, preferredRunID, vpath)
	}
	if teamID := strings.TrimSpace(scope.teamID); teamID != "" {
		return resolveArtifactDiskPath(s.cfg.DataDir, teamID, scope.runID, vpath)
	}
	if runID := strings.TrimSpace(preferredRunID); runID != "" {
		if p := resolveArtifactDiskPath(s.cfg.DataDir, "", runID, vpath); p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	for _, runID := range s.sessionRunIDs(ctx, scope.sessionID, scope.runID) {
		if p := resolveArtifactDiskPath(s.cfg.DataDir, "", runID, vpath); p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

func (s *RPCServer) artifactGet(ctx context.Context, p protocol.ArtifactGetParams) (protocol.ArtifactGetResult, error) {
	scope, err := s.resolveArtifactScope(ctx, p.ThreadID, p.TeamID)
	if err != nil {
		return protocol.ArtifactGetResult{}, err
	}
	artifactID := strings.TrimSpace(p.ArtifactID)
	vpath := strings.TrimSpace(p.VPath)
	if vpath == "" && (strings.HasPrefix(artifactID, "file:/workspace/") || strings.HasPrefix(artifactID, "file:/tasks/") || strings.HasPrefix(artifactID, "file:/deliverables/")) {
		vpath = strings.TrimPrefix(artifactID, "file:")
	}
	if artifactID == "" && vpath == "" {
		return protocol.ArtifactGetResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "artifactId or vpath is required"}
	}

	indexer, err := s.artifactIndexer()
	if err != nil {
		return protocol.ArtifactGetResult{}, err
	}
	groups, err := s.listArtifactGroupsForScope(ctx, indexer, scope, state.ArtifactFilter{
		Limit: artifactListMaxLimit,
	})
	if err != nil {
		return protocol.ArtifactGetResult{}, err
	}

	var sel state.ArtifactRecord
	var parent state.ArtifactGroup
	found := false
	for _, g := range groups {
		for _, f := range g.Files {
			if artifactID != "" && strings.TrimSpace(f.ArtifactID) == artifactID {
				sel, parent, found = f, g, true
				break
			}
			if vpath != "" && strings.TrimSpace(f.VPath) == vpath {
				sel, parent, found = f, g, true
				break
			}
		}
		if found {
			break
		}
	}
	maxBytes := clampLimit(p.MaxBytes, artifactGetDefaultBytes, artifactGetMaxBytes)
	if !found && strings.TrimSpace(vpath) != "" {
		diskPath := s.resolveArtifactDiskPathForScope(ctx, scope, "", "", vpath)
		if strings.TrimSpace(diskPath) == "" {
			return protocol.ArtifactGetResult{}, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "artifact not found"}
		}
		f, err := os.Open(diskPath)
		if err != nil {
			return protocol.ArtifactGetResult{}, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "artifact file not found"}
		}
		defer f.Close()
		buf, err := io.ReadAll(io.LimitReader(f, int64(maxBytes)+1))
		if err != nil {
			return protocol.ArtifactGetResult{}, err
		}
		truncated := len(buf) > maxBytes
		if truncated {
			buf = buf[:maxBytes]
		}
		fileLabel := filepath.Base(strings.TrimSpace(vpath))
		if fileLabel == "" || fileLabel == "." || fileLabel == "/" {
			fileLabel = strings.TrimSpace(vpath)
		}
		node := protocol.ArtifactNode{
			NodeKey:     "file:" + strings.TrimSpace(vpath),
			Kind:        "file",
			Label:       fileLabel,
			ArtifactID:  strings.TrimSpace(artifactID),
			DisplayName: fileLabel,
			VPath:       strings.TrimSpace(vpath),
			DiskPath:    diskPath,
		}
		return protocol.ArtifactGetResult{
			Artifact:  node,
			Content:   string(buf),
			Truncated: truncated,
			BytesRead: len(buf),
		}, nil
	}
	if !found {
		return protocol.ArtifactGetResult{}, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "artifact not found"}
	}

	diskPath := strings.TrimSpace(sel.DiskPath)
	if diskPath == "" || func() bool { _, err := os.Stat(diskPath); return err != nil }() {
		diskPath = s.resolveArtifactDiskPathForScope(ctx, scope, sel.TeamID, sel.RunID, sel.VPath)
	}
	if strings.TrimSpace(diskPath) == "" {
		return protocol.ArtifactGetResult{}, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "artifact file path unavailable"}
	}
	f, err := os.Open(diskPath)
	if err != nil {
		return protocol.ArtifactGetResult{}, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "artifact file not found"}
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, int64(maxBytes)+1))
	if err != nil {
		return protocol.ArtifactGetResult{}, err
	}
	truncated := len(buf) > maxBytes
	if truncated {
		buf = buf[:maxBytes]
	}
	node := protocol.ArtifactNode{
		NodeKey:     "file:" + strings.TrimSpace(sel.VPath),
		ParentKey:   "task:" + strings.TrimSpace(parent.TaskID),
		Kind:        "file",
		Label:       strings.TrimSpace(sel.DisplayName),
		DayBucket:   strings.TrimSpace(parent.DayBucket),
		Role:        strings.TrimSpace(parent.Role),
		TaskKind:    strings.TrimSpace(parent.TaskKind),
		TaskID:      strings.TrimSpace(parent.TaskID),
		Status:      strings.TrimSpace(parent.Status),
		ArtifactID:  strings.TrimSpace(sel.ArtifactID),
		DisplayName: strings.TrimSpace(sel.DisplayName),
		VPath:       strings.TrimSpace(sel.VPath),
		DiskPath:    diskPath,
		IsSummary:   sel.IsSummary,
		ProducedAt:  sel.ProducedAt,
	}
	if node.Label == "" {
		node.Label = filepath.Base(node.VPath)
	}
	return protocol.ArtifactGetResult{
		Artifact:  node,
		Content:   string(buf),
		Truncated: truncated,
		BytesRead: len(buf),
	}, nil
}

func taskKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case state.TaskKindCallback:
		return "Callback Tasks"
	case state.TaskKindHeartbeat:
		return "Heartbeat Tasks"
	case state.TaskKindCoordinator:
		return "Coordinator Tasks"
	case state.TaskKindTask:
		return "Tasks"
	default:
		return "Other Tasks"
	}
}

func artifactGroupsToNodes(groups []state.ArtifactGroup) []protocol.ArtifactNode {
	out := make([]protocol.ArtifactNode, 0, len(groups)*6)
	lastDay := ""
	lastRole := ""
	lastKind := ""
	for _, g := range groups {
		day := strings.TrimSpace(g.DayBucket)
		role := strings.TrimSpace(g.Role)
		kind := strings.TrimSpace(g.TaskKind)
		taskID := strings.TrimSpace(g.TaskID)
		if taskID == "" {
			continue
		}
		dayKey := "day:" + day
		roleKey := "role:" + day + ":" + role
		kindKey := "kind:" + day + ":" + role + ":" + kind
		taskKey := "task:" + taskID

		if day != lastDay {
			lastDay = day
			lastRole = ""
			lastKind = ""
			out = append(out, protocol.ArtifactNode{
				NodeKey:   dayKey,
				Kind:      "day",
				Label:     day,
				DayBucket: day,
			})
		}
		if role != lastRole {
			lastRole = role
			lastKind = ""
			out = append(out, protocol.ArtifactNode{
				NodeKey:   roleKey,
				ParentKey: dayKey,
				Kind:      "role",
				Label:     role,
				DayBucket: day,
				Role:      role,
			})
		}
		if kind != lastKind {
			lastKind = kind
			out = append(out, protocol.ArtifactNode{
				NodeKey:   kindKey,
				ParentKey: roleKey,
				Kind:      "stream",
				Label:     taskKindLabel(kind),
				DayBucket: day,
				Role:      role,
				TaskKind:  kind,
			})
		}
		label := strings.TrimSpace(g.Goal)
		if label == "" {
			label = taskID
		}
		out = append(out, protocol.ArtifactNode{
			NodeKey:   taskKey,
			ParentKey: kindKey,
			Kind:      "task",
			Label:     label,
			DayBucket: day,
			Role:      role,
			TaskKind:  kind,
			TaskID:    taskID,
			Status:    strings.TrimSpace(g.Status),
		})
		for _, f := range g.Files {
			fileLabel := strings.TrimSpace(f.DisplayName)
			if fileLabel == "" {
				fileLabel = filepath.Base(strings.TrimSpace(f.VPath))
			}
			out = append(out, protocol.ArtifactNode{
				NodeKey:     "file:" + strings.TrimSpace(f.VPath),
				ParentKey:   taskKey,
				Kind:        "file",
				Label:       fileLabel,
				DayBucket:   day,
				Role:        role,
				TaskKind:    kind,
				TaskID:      taskID,
				Status:      strings.TrimSpace(g.Status),
				ArtifactID:  strings.TrimSpace(f.ArtifactID),
				DisplayName: strings.TrimSpace(f.DisplayName),
				VPath:       strings.TrimSpace(f.VPath),
				DiskPath:    strings.TrimSpace(f.DiskPath),
				IsSummary:   f.IsSummary,
				ProducedAt:  f.ProducedAt,
			})
		}
	}
	return out
}

func searchMatchesToNodes(matches []state.ArtifactRecord, byTask map[string]state.ArtifactGroup) []protocol.ArtifactNode {
	out := make([]protocol.ArtifactNode, 0, len(matches)*5)
	seen := map[string]struct{}{}
	add := func(n protocol.ArtifactNode) {
		if strings.TrimSpace(n.NodeKey) == "" {
			return
		}
		if _, ok := seen[n.NodeKey]; ok {
			return
		}
		seen[n.NodeKey] = struct{}{}
		out = append(out, n)
	}
	for _, f := range matches {
		taskID := strings.TrimSpace(f.TaskID)
		g := byTask[taskID]
		day := strings.TrimSpace(g.DayBucket)
		role := strings.TrimSpace(g.Role)
		kind := strings.TrimSpace(g.TaskKind)
		if day == "" {
			day = strings.TrimSpace(f.DayBucket)
		}
		if role == "" {
			role = strings.TrimSpace(f.Role)
		}
		if kind == "" {
			kind = strings.TrimSpace(f.TaskKind)
		}
		dayKey := "day:" + day
		roleKey := "role:" + day + ":" + role
		kindKey := "kind:" + day + ":" + role + ":" + kind
		taskKey := "task:" + taskID
		add(protocol.ArtifactNode{NodeKey: dayKey, Kind: "day", Label: day, DayBucket: day})
		add(protocol.ArtifactNode{NodeKey: roleKey, ParentKey: dayKey, Kind: "role", Label: role, DayBucket: day, Role: role})
		add(protocol.ArtifactNode{NodeKey: kindKey, ParentKey: roleKey, Kind: "stream", Label: taskKindLabel(kind), DayBucket: day, Role: role, TaskKind: kind})
		taskLabel := strings.TrimSpace(g.Goal)
		if taskLabel == "" {
			taskLabel = taskID
		}
		add(protocol.ArtifactNode{
			NodeKey: taskKey, ParentKey: kindKey, Kind: "task", Label: taskLabel,
			DayBucket: day, Role: role, TaskKind: kind, TaskID: taskID, Status: strings.TrimSpace(g.Status),
		})
		fileLabel := strings.TrimSpace(f.DisplayName)
		if fileLabel == "" {
			fileLabel = filepath.Base(strings.TrimSpace(f.VPath))
		}
		add(protocol.ArtifactNode{
			NodeKey: "file:" + strings.TrimSpace(f.VPath), ParentKey: taskKey, Kind: "file", Label: fileLabel,
			DayBucket: day, Role: role, TaskKind: kind, TaskID: taskID, Status: strings.TrimSpace(g.Status),
			ArtifactID: strings.TrimSpace(f.ArtifactID), DisplayName: strings.TrimSpace(f.DisplayName),
			VPath: strings.TrimSpace(f.VPath), DiskPath: strings.TrimSpace(f.DiskPath), IsSummary: f.IsSummary, ProducedAt: f.ProducedAt,
		})
	}
	return out
}
