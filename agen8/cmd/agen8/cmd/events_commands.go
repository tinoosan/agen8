package cmd

import (
	"context"
	"fmt"
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
	activityFollow    bool
	activityLimit     int
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Query structured run events",
	RunE: func(cmd *cobra.Command, args []string) error {
		runID, err := resolveTargetRunID(cmd.Context(), strings.TrimSpace(logsRunID), strings.TrimSpace(logsSessionID))
		if err != nil {
			return err
		}
		if aid := strings.TrimSpace(logsAgentID); aid != "" {
			runID = aid
		}
		typesFilter := normalizeTypeFilter(logsTypes)
		if !logsFollow {
			_, err := printLogsBatch(cmd, runID, typesFilter, int64(0), logsLimit)
			return err
		}
		var cursor int64
		for {
			next, err := printLogsBatch(cmd, runID, typesFilter, cursor, logsLimit)
			if err != nil {
				return err
			}
			if next > 0 {
				cursor = next
			} else {
				var latest protocol.EventsLatestSeqResult
				if err := rpcCall(cmd.Context(), protocol.MethodEventsLatestSeq, protocol.EventsLatestSeqParams{RunID: runID}, &latest); err == nil {
					cursor = latest.Seq
				}
			}
			time.Sleep(1200 * time.Millisecond)
		}
	},
}

var activityCmd = &cobra.Command{
	Use:   "activity",
	Short: "Tail live activity for a run",
	RunE: func(cmd *cobra.Command, args []string) error {
		runID, err := resolveTargetRunID(cmd.Context(), strings.TrimSpace(activityRunID), strings.TrimSpace(activitySessionID))
		if err != nil {
			return err
		}
		limit := activityLimit
		if limit <= 0 {
			limit = 50
		}
		if !activityFollow {
			_, err := printActivityBatch(cmd, runID, int64(0), limit)
			return err
		}
		var cursor int64
		for {
			next, err := printActivityBatch(cmd, runID, cursor, limit)
			if err != nil {
				return err
			}
			if next > 0 {
				cursor = next
			} else {
				var latest protocol.EventsLatestSeqResult
				if err := rpcCall(cmd.Context(), protocol.MethodEventsLatestSeq, protocol.EventsLatestSeqParams{RunID: runID}, &latest); err == nil {
					cursor = latest.Seq
				}
			}
			time.Sleep(1 * time.Second)
		}
	},
}

func printLogsBatch(cmd *cobra.Command, runID string, typesFilter []string, afterSeq int64, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
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
	if limit <= 0 {
		limit = 100
	}
	var out protocol.ActivityStreamResult
	if err := rpcCall(cmd.Context(), protocol.MethodActivityStream, protocol.ActivityStreamParams{
		RunID:    runID,
		AfterSeq: afterSeq,
		Limit:    limit,
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

func formatEventLine(ev types.EventRecord) string {
	ts := ev.Timestamp.UTC().Format(time.RFC3339)
	msg := strings.TrimSpace(ev.Message)
	if msg == "" {
		msg = "-"
	}
	return fmt.Sprintf("%s  %s  %s  %s", ts, strings.TrimSpace(ev.RunID), strings.TrimSpace(ev.Type), msg)
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

func resolveTargetRunID(ctx context.Context, runID string, sessionID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID != "" {
		return runID, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		projectCtx, err := loadProjectContext()
		if err == nil && projectCtx.Exists {
			if rid := strings.TrimSpace(projectCtx.State.ActiveRunID); rid != "" {
				return rid, nil
			}
			sessionID = strings.TrimSpace(projectCtx.State.ActiveSessionID)
		}
	}
	if sessionID == "" {
		return "", fmt.Errorf("run id required (use --run-id or --session-id, or attach a project session)")
	}
	resolvedRunID, _, err := rpcResolveCoordinatorRun(ctx, sessionID)
	if err != nil {
		return "", err
	}
	return resolvedRunID, nil
}

func init() {
	logsCmd.Flags().StringVar(&logsRunID, "run-id", "", "run id to query")
	logsCmd.Flags().StringVar(&logsSessionID, "session-id", "", "resolve run id from session")
	logsCmd.Flags().StringVar(&logsAgentID, "agent", "", "agent/run id alias for filtering")
	logsCmd.Flags().StringSliceVar(&logsTypes, "type", nil, "event type filter (repeat or comma-separated)")
	logsCmd.Flags().BoolVar(&logsFollow, "follow", false, "follow log updates")
	logsCmd.Flags().IntVar(&logsLimit, "limit", 200, "max events per poll")

	activityCmd.Flags().StringVar(&activityRunID, "run-id", "", "run id to tail")
	activityCmd.Flags().StringVar(&activitySessionID, "session-id", "", "resolve run id from session")
	activityCmd.Flags().BoolVar(&activityFollow, "follow", true, "follow activity stream")
	activityCmd.Flags().IntVar(&activityLimit, "limit", 100, "max events per poll")

	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(activityCmd)
}
