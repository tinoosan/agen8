package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
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
	if err := addBoundHandler[protocol.ArtifactListParams, protocol.ArtifactListResult](reg, protocol.MethodArtifactList, false, s.artifactList); err != nil {
		return err
	}
	if err := addBoundHandler[protocol.ArtifactSearchParams, protocol.ArtifactSearchResult](reg, protocol.MethodArtifactSearch, false, s.artifactSearch); err != nil {
		return err
	}
	if err := addBoundHandler[protocol.ArtifactGetParams, protocol.ArtifactGetResult](reg, protocol.MethodArtifactGet, false, s.artifactGet); err != nil {
		return err
	}
	return nil
}

type artifactScope struct {
	sessionID string
	teamID    string
	runID     string
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
	if teamID == "" && s.taskStore != nil && strings.TrimSpace(scope.runID) != "" {
		tasks, err := s.taskStore.ListTasks(ctx, state.TaskFilter{
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
	if s == nil || s.taskStore == nil {
		return nil, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "task store not configured"}
	}
	indexer, ok := s.taskStore.(state.ArtifactIndexer)
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
	groups, err := indexer.ListArtifactGroups(ctx, state.ArtifactFilter{
		TeamID:    scope.teamID,
		RunID:     scope.runID,
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
		TeamID:    scope.teamID,
		RunID:     scope.runID,
		DayBucket: strings.TrimSpace(p.DayBucket),
		Role:      strings.TrimSpace(p.Role),
		TaskKind:  strings.TrimSpace(p.TaskKind),
		TaskID:    strings.TrimSpace(p.TaskID),
		Limit:     clampLimit(p.Limit, artifactSearchDefault, artifactSearchMax),
	}
	applyScopeKey(&filter, p.ScopeKey)
	matches, err := indexer.SearchArtifacts(ctx, state.ArtifactSearchFilter{
		ArtifactFilter: filter,
		Query:          query,
	})
	if err != nil {
		return protocol.ArtifactSearchResult{}, err
	}
	if len(matches) == 0 {
		return protocol.ArtifactSearchResult{Nodes: nil, MatchCount: 0}, nil
	}
	groups, err := indexer.ListArtifactGroups(ctx, state.ArtifactFilter{
		TeamID:    filter.TeamID,
		RunID:     filter.RunID,
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
	if !strings.HasPrefix(vpath, "/workspace/") {
		return ""
	}
	rel := strings.TrimPrefix(vpath, "/workspace/")
	resolveLegacy := func(base string) string {
		legacyRel := strings.TrimPrefix(rel, "tasks/")
		candidate := filepath.Join(base, "deliverables", legacyRel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		return ""
	}
	if strings.TrimSpace(teamID) != "" {
		base := fsutil.GetTeamWorkspaceDir(dataDir, teamID)
		candidate := filepath.Join(base, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		if strings.HasPrefix(rel, "tasks/") {
			if legacy := resolveLegacy(base); legacy != "" {
				return legacy
			}
		}
		return candidate
	}
	base := fsutil.GetWorkspaceDir(dataDir, runID)
	candidate := filepath.Join(base, rel)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	if strings.HasPrefix(rel, "tasks/") {
		if legacy := resolveLegacy(base); legacy != "" {
			return legacy
		}
	}
	return candidate
}

func (s *RPCServer) artifactGet(ctx context.Context, p protocol.ArtifactGetParams) (protocol.ArtifactGetResult, error) {
	scope, err := s.resolveArtifactScope(ctx, p.ThreadID, p.TeamID)
	if err != nil {
		return protocol.ArtifactGetResult{}, err
	}
	artifactID := strings.TrimSpace(p.ArtifactID)
	vpath := strings.TrimSpace(p.VPath)
	if artifactID == "" && vpath == "" {
		return protocol.ArtifactGetResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "artifactId or vpath is required"}
	}

	indexer, err := s.artifactIndexer()
	if err != nil {
		return protocol.ArtifactGetResult{}, err
	}
	groups, err := indexer.ListArtifactGroups(ctx, state.ArtifactFilter{
		TeamID: scope.teamID,
		RunID:  scope.runID,
		Limit:  artifactListMaxLimit,
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
	if !found {
		return protocol.ArtifactGetResult{}, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "artifact not found"}
	}

	maxBytes := clampLimit(p.MaxBytes, artifactGetDefaultBytes, artifactGetMaxBytes)
	diskPath := strings.TrimSpace(sel.DiskPath)
	if diskPath == "" {
		diskPath = resolveArtifactDiskPath(s.cfg.DataDir, scope.teamID, scope.runID, sel.VPath)
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
