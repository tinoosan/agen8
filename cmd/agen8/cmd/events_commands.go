package cmd

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
)

var (
	logsTeamID    string
	logsRunID     string
	logsSessionID string
	logsAgentID   string
	logsTypes     []string
	logsFollow    bool
	logsLimit     int
)

var logsListProjectSessionsFn = listProjectSessionIDs
var logsListSessionRunIDsFn = listSessionRunIDs
var logsListProjectTeamRunIDsFn = listProjectTeamRunIDs
var logsListSessionAgentsFn = listSessionAgents

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Live structured logs across the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		teamID := strings.TrimSpace(logsTeamID)
		sessionID := strings.TrimSpace(logsSessionID)
		runID := strings.TrimSpace(logsRunID)
		agentID := strings.TrimSpace(logsAgentID)
		runIDs, err := resolveTargetRunIDs(
			cmd.Context(),
			runID,
			sessionID,
			agentID,
			teamID,
		)
		if err != nil {
			return err
		}
		runRoles, err := resolveRunRoleLabels(cmd.Context(), runIDs, sessionID, teamID)
		if err != nil {
			return err
		}
		typesFilter := normalizeTypeFilter(logsTypes)
		return followEventRuns(cmd, runIDs, logsFollow, logsLimit, func(cmd *cobra.Command, runID string, afterSeq int64, limit int) (int64, error) {
			return printLogsBatch(cmd, runID, runRoles[runID], typesFilter, afterSeq, limit)
		})
	},
}

func followEventRuns(cmd *cobra.Command, runIDs []string, follow bool, limit int, printer func(cmd *cobra.Command, runID string, afterSeq int64, limit int) (int64, error)) error {
	if len(runIDs) == 0 {
		return fmt.Errorf("no runs found for selected scope")
	}
	if limit <= 0 {
		limit = 100
	}
	cursors := map[string]int64{}
	retries := 0
	for {
		progress := false
		for _, runID := range runIDs {
			next, err := printer(cmd, runID, cursors[runID], limit)
			if err != nil {
				if isRetryableLiveError(err) {
					retries++
					backoff := time.Duration(minInt(8, retries)) * 300 * time.Millisecond
					fmt.Fprintf(cmd.ErrOrStderr(), "stream reconnecting (%v)\n", err)
					time.Sleep(backoff)
					continue
				}
				return err
			}
			if next > cursors[runID] {
				progress = true
				cursors[runID] = next
			}
		}
		if !follow {
			return nil
		}
		if !progress {
			time.Sleep(1200 * time.Millisecond)
		}
	}
}

func printLogsBatch(cmd *cobra.Command, runID string, role string, typesFilter []string, afterSeq int64, limit int) (int64, error) {
	var out protocol.LogsQueryResult
	if err := rpcCall(cmd.Context(), protocol.MethodLogsQuery, protocol.LogsQueryParams{
		RunID:    runID,
		AfterSeq: afterSeq,
		Limit:    limit,
		Types:    typesFilter,
		SortDesc: false,
	}, &out); err != nil {
		return afterSeq, err
	}
	for _, ev := range out.Events {
		fmt.Fprintln(cmd.OutOrStdout(), formatEventLine(ev, role))
	}
	if out.Next > 0 {
		return out.Next, nil
	}
	return afterSeq, nil
}

func formatEventLine(ev types.EventRecord, role string) string {
	ts := ev.Timestamp.UTC().Format(time.RFC3339)
	msg := strings.TrimSpace(ev.Message)
	if msg == "" {
		msg = "-"
	}
	return fmt.Sprintf("%s  %s  %s  %s  %s", ts, blankDash(strings.TrimSpace(ev.RunID)), blankDash(role), strings.TrimSpace(ev.Type), msg)
}

func normalizeTypeFilter(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, ",") {
			parts := strings.Split(item, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				if _, ok := seen[p]; ok {
					continue
				}
				seen[p] = struct{}{}
				out = append(out, p)
			}
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func resolveTargetRunIDs(ctx context.Context, runID string, sessionID string, agent string, teamID string) ([]string, error) {
	if runID = strings.TrimSpace(runID); runID != "" {
		return []string{runID}, nil
	}
	if agent = strings.TrimSpace(agent); agent != "" {
		return []string{agent}, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		projectCtx, err := loadProjectContext()
		if err == nil && projectCtx.Exists {
			projectRoot := strings.TrimSpace(projectCtx.RootDir)
			if projectRoot != "" {
				if teamID != "" {
					runIDs, err := logsListProjectTeamRunIDsFn(ctx, projectRoot, teamID)
					if err != nil {
						return nil, err
					}
					if len(runIDs) > 0 {
						return runIDs, nil
					}
				} else {
					sessionIDs, err := logsListProjectSessionsFn(ctx, projectRoot)
					if err == nil && len(sessionIDs) > 0 {
						runIDs := make([]string, 0, len(sessionIDs))
						seen := map[string]struct{}{}
						for _, sessionID := range sessionIDs {
							ids, err := logsListSessionRunIDsFn(ctx, sessionID)
							if err != nil {
								return nil, err
							}
							for _, rid := range ids {
								rid = strings.TrimSpace(rid)
								if rid == "" {
									continue
								}
								if _, ok := seen[rid]; ok {
									continue
								}
								seen[rid] = struct{}{}
								runIDs = append(runIDs, rid)
							}
						}
						slices.Sort(runIDs)
						if len(runIDs) > 0 {
							return runIDs, nil
						}
					}
				}
			}
			sessionID = strings.TrimSpace(projectCtx.State.ActiveSessionID)
		}
	}
	if sessionID == "" {
		if teamID != "" {
			return nil, fmt.Errorf("no runs found for team %q in the current project", teamID)
		}
		return nil, fmt.Errorf("no logs scope found (initialize a project or pass --team-id)")
	}
	return logsListSessionRunIDsFn(ctx, sessionID)
}

func listProjectSessionIDs(ctx context.Context, projectRoot string) ([]string, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil, nil
	}
	var out protocol.SessionListResult
	if err := rpcCall(ctx, protocol.MethodSessionList, protocol.SessionListParams{
		ThreadID:    detachedThreadID,
		ProjectRoot: projectRoot,
		Limit:       500,
		Offset:      0,
	}, &out); err != nil {
		return nil, err
	}
	sessionIDs := make([]string, 0, len(out.Sessions))
	for _, item := range out.Sessions {
		sessionID := strings.TrimSpace(item.SessionID)
		if sessionID == "" {
			continue
		}
		sessionIDs = append(sessionIDs, sessionID)
	}
	return sessionIDs, nil
}

func listSessionRunIDs(ctx context.Context, sessionID string) ([]string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	var agents protocol.AgentListResult
	if err := rpcCall(ctx, protocol.MethodAgentList, protocol.AgentListParams{
		ThreadID:  protocol.ThreadID(sessionID),
		SessionID: sessionID,
	}, &agents); err != nil {
		return nil, err
	}
	runIDs := make([]string, 0, len(agents.Agents)+1)
	for _, agent := range agents.Agents {
		rid := strings.TrimSpace(agent.RunID)
		if rid == "" || slices.Contains(runIDs, rid) {
			continue
		}
		runIDs = append(runIDs, rid)
	}
	if len(runIDs) == 0 {
		resolvedRunID, _, err := rpcResolveCoordinatorRun(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(resolvedRunID) != "" {
			runIDs = append(runIDs, strings.TrimSpace(resolvedRunID))
		}
	}
	slices.Sort(runIDs)
	return runIDs, nil
}

func listSessionAgents(ctx context.Context, sessionID string) ([]protocol.AgentListItem, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	var agents protocol.AgentListResult
	if err := rpcCall(ctx, protocol.MethodAgentList, protocol.AgentListParams{
		ThreadID:  protocol.ThreadID(sessionID),
		SessionID: sessionID,
	}, &agents); err != nil {
		return nil, err
	}
	return agents.Agents, nil
}

func resolveRunRoleLabels(ctx context.Context, runIDs []string, sessionID string, teamID string) (map[string]string, error) {
	runSet := map[string]struct{}{}
	for _, runID := range runIDs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		runSet[runID] = struct{}{}
	}
	if len(runSet) == 0 {
		return map[string]string{}, nil
	}

	sessionIDs := make([]string, 0, 8)
	seenSessions := map[string]struct{}{}
	appendSessionID := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seenSessions[id]; ok {
			return
		}
		seenSessions[id] = struct{}{}
		sessionIDs = append(sessionIDs, id)
	}

	appendSessionID(sessionID)
	if len(sessionIDs) == 0 {
		projectCtx, err := loadProjectContext()
		if err == nil && projectCtx.Exists {
			projectRoot := strings.TrimSpace(projectCtx.RootDir)
			if projectRoot != "" {
				projectSessionIDs, err := logsListProjectSessionsFn(ctx, projectRoot)
				if err != nil {
					return nil, err
				}
				for _, projectSessionID := range projectSessionIDs {
					appendSessionID(projectSessionID)
				}
			}
		}
	}

	rolesByRunID := map[string]string{}
	for _, sessionID := range sessionIDs {
		agents, err := logsListSessionAgentsFn(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		for _, agent := range agents {
			runID := strings.TrimSpace(agent.RunID)
			if _, ok := runSet[runID]; !ok {
				continue
			}
			if teamID != "" && strings.TrimSpace(agent.TeamID) != teamID {
				continue
			}
			if _, ok := rolesByRunID[runID]; ok {
				continue
			}
			rolesByRunID[runID] = strings.TrimSpace(agent.Role)
		}
	}
	return rolesByRunID, nil
}

func listProjectTeamRunIDs(ctx context.Context, projectRoot string, teamID string) ([]string, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	teamID = strings.TrimSpace(teamID)
	if projectRoot == "" || teamID == "" {
		return nil, nil
	}
	var out protocol.SessionListResult
	if err := rpcCall(ctx, protocol.MethodSessionList, protocol.SessionListParams{
		ThreadID:    detachedThreadID,
		ProjectRoot: projectRoot,
		Limit:       500,
		Offset:      0,
	}, &out); err != nil {
		return nil, err
	}
	runIDs := make([]string, 0, len(out.Sessions))
	for _, item := range out.Sessions {
		if strings.TrimSpace(item.TeamID) != teamID {
			continue
		}
		sessionRunIDs, err := logsListSessionRunIDsFn(ctx, strings.TrimSpace(item.SessionID))
		if err != nil {
			return nil, err
		}
		for _, runID := range sessionRunIDs {
			runID = strings.TrimSpace(runID)
			if runID == "" || slices.Contains(runIDs, runID) {
				continue
			}
			runIDs = append(runIDs, runID)
		}
	}
	slices.Sort(runIDs)
	return runIDs, nil
}

func init() {
	logsCmd.Flags().StringVar(&logsTeamID, "team-id", "", "team id scope within the current project")
	logsCmd.Flags().StringVar(&logsRunID, "run-id", "", "run id to query")
	logsCmd.Flags().StringVar(&logsSessionID, "session-id", "", "session id scope (defaults to the current project)")
	logsCmd.Flags().StringVar(&logsAgentID, "agent", "", "agent/run id alias for filtering")
	_ = logsCmd.Flags().MarkHidden("run-id")
	_ = logsCmd.Flags().MarkHidden("session-id")
	_ = logsCmd.Flags().MarkHidden("agent")
	logsCmd.Flags().StringSliceVar(&logsTypes, "type", nil, "event type filter (repeat or comma-separated)")
	logsCmd.Flags().BoolVar(&logsFollow, "follow", true, "follow log updates")
	logsCmd.Flags().IntVar(&logsLimit, "limit", 200, "max events per poll per run")

	rootCmd.AddCommand(logsCmd)
}
