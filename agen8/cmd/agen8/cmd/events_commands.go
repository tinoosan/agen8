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
	logsRunID     string
	logsSessionID string
	logsAgentID   string
	logsTypes     []string
	logsFollow    bool
	logsLimit     int

	activityRunID     string
	activitySessionID string
	activityAgentID   string
	activityRole      string
	activityFollow    bool
	activityLimit     int
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Live structured logs (daemon/session/agent events)",
	RunE: func(cmd *cobra.Command, args []string) error {
		runIDs, err := resolveTargetRunIDs(cmd.Context(), strings.TrimSpace(logsRunID), strings.TrimSpace(logsSessionID), strings.TrimSpace(logsAgentID))
		if err != nil {
			return err
		}
		typesFilter := normalizeTypeFilter(logsTypes)
		return followEventRuns(cmd, runIDs, logsFollow, logsLimit, func(cmd *cobra.Command, runID string, afterSeq int64, limit int) (int64, error) {
			return printLogsBatch(cmd, runID, typesFilter, afterSeq, limit)
		})
	},
}

var activityCmd = &cobra.Command{
	Use:   "activity",
	Short: "Live agent activity stream (tool calls, delegation, callbacks)",
	RunE: func(cmd *cobra.Command, args []string) error {
		runIDs, err := resolveTargetRunIDs(cmd.Context(), strings.TrimSpace(activityRunID), strings.TrimSpace(activitySessionID), strings.TrimSpace(activityAgentID))
		if err != nil {
			return err
		}
		if role := strings.TrimSpace(activityRole); role != "" {
			filtered, ferr := filterRunIDsByRole(cmd.Context(), runIDs, strings.TrimSpace(activitySessionID), role)
			if ferr != nil {
				return ferr
			}
			runIDs = filtered
		}
		return followEventRuns(cmd, runIDs, activityFollow, activityLimit, func(cmd *cobra.Command, runID string, afterSeq int64, limit int) (int64, error) {
			return printActivityBatch(cmd, runID, afterSeq, limit)
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

func printLogsBatch(cmd *cobra.Command, runID string, typesFilter []string, afterSeq int64, limit int) (int64, error) {
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
		fmt.Fprintln(cmd.OutOrStdout(), formatEventLine(ev))
	}
	if out.Next > 0 {
		return out.Next, nil
	}
	return afterSeq, nil
}

func printActivityBatch(cmd *cobra.Command, runID string, afterSeq int64, limit int) (int64, error) {
	var out protocol.LogsQueryResult
	if err := rpcCall(cmd.Context(), protocol.MethodLogsQuery, protocol.LogsQueryParams{
		RunID:    runID,
		AfterSeq: afterSeq,
		Limit:    limit,
		SortDesc: false,
	}, &out); err != nil {
		return afterSeq, err
	}
	for _, ev := range out.Events {
		if !isActivityEvent(ev) {
			continue
		}
		fmt.Fprintln(cmd.OutOrStdout(), formatActivityLine(ev))
	}
	if out.Next > 0 {
		return out.Next, nil
	}
	return afterSeq, nil
}

func formatEventLine(ev types.EventRecord) string {
	ts := ev.Timestamp.UTC().Format(time.RFC3339)
	msg := strings.TrimSpace(ev.Message)
	if msg == "" {
		msg = "-"
	}
	return fmt.Sprintf("%s  %s  %s  %s", ts, strings.TrimSpace(ev.RunID), strings.TrimSpace(ev.Type), msg)
}

func formatActivityLine(ev types.EventRecord) string {
	ts := ev.Timestamp.UTC().Format(time.RFC3339)
	op := strings.TrimSpace(ev.Data["op"])
	if op == "" {
		op = strings.TrimSpace(ev.Data["tool"])
	}
	if op == "" {
		op = strings.TrimSpace(ev.Type)
	}
	msg := strings.TrimSpace(ev.Message)
	if msg == "" {
		msg = strings.TrimSpace(ev.Data["summary"])
	}
	return fmt.Sprintf("%s  %s  %s  %s", ts, strings.TrimSpace(ev.RunID), op, fallback(msg, "-"))
}

func isActivityEvent(ev types.EventRecord) bool {
	evType := strings.ToLower(strings.TrimSpace(ev.Type))
	if evType == "" {
		return false
	}
	if strings.Contains(evType, "host.op.request") || strings.Contains(evType, "host.op.response") {
		return true
	}
	if strings.Contains(evType, "task.create") || strings.Contains(evType, "task.claim") || strings.Contains(evType, "task.complete") {
		return true
	}
	if strings.Contains(evType, "callback") || strings.Contains(evType, "review") || strings.Contains(evType, "delegate") {
		return true
	}
	if strings.TrimSpace(ev.Data["op"]) != "" || strings.TrimSpace(ev.Data["tool"]) != "" {
		return true
	}
	return false
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

func resolveTargetRunIDs(ctx context.Context, runID string, sessionID string, agent string) ([]string, error) {
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
			if rid := strings.TrimSpace(projectCtx.State.ActiveRunID); rid != "" {
				return []string{rid}, nil
			}
			sessionID = strings.TrimSpace(projectCtx.State.ActiveSessionID)
		}
	}
	if sessionID == "" {
		return nil, fmt.Errorf("run id required (use --run-id or --session-id, or attach a project session)")
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
	return runIDs, nil
}

func filterRunIDsByRole(ctx context.Context, runIDs []string, sessionID string, role string) ([]string, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		return runIDs, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return runIDs, nil
	}
	var agents protocol.AgentListResult
	if err := rpcCall(ctx, protocol.MethodAgentList, protocol.AgentListParams{
		ThreadID:  protocol.ThreadID(sessionID),
		SessionID: sessionID,
	}, &agents); err != nil {
		return nil, err
	}
	filtered := make([]string, 0, len(runIDs))
	for _, agent := range agents.Agents {
		if !strings.EqualFold(strings.TrimSpace(agent.Role), role) {
			continue
		}
		rid := strings.TrimSpace(agent.RunID)
		if rid == "" || !slices.Contains(runIDs, rid) {
			continue
		}
		filtered = append(filtered, rid)
	}
	return filtered, nil
}

func init() {
	logsCmd.Flags().StringVar(&logsRunID, "run-id", "", "run id to query")
	logsCmd.Flags().StringVar(&logsSessionID, "session-id", "", "session id scope (defaults to active project session)")
	logsCmd.Flags().StringVar(&logsAgentID, "agent", "", "agent/run id alias for filtering")
	logsCmd.Flags().StringSliceVar(&logsTypes, "type", nil, "event type filter (repeat or comma-separated)")
	logsCmd.Flags().BoolVar(&logsFollow, "follow", true, "follow log updates")
	logsCmd.Flags().IntVar(&logsLimit, "limit", 200, "max events per poll per run")

	activityCmd.Flags().StringVar(&activityRunID, "run-id", "", "run id to tail")
	activityCmd.Flags().StringVar(&activitySessionID, "session-id", "", "session id scope (defaults to active project session)")
	activityCmd.Flags().StringVar(&activityAgentID, "agent", "", "agent/run id filter")
	activityCmd.Flags().StringVar(&activityRole, "role", "", "role filter for session activity")
	activityCmd.Flags().BoolVar(&activityFollow, "follow", true, "follow activity stream")
	activityCmd.Flags().IntVar(&activityLimit, "limit", 100, "max events per poll per run")

	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(activityCmd)
}
