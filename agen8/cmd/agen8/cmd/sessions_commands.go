package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/pkg/protocol"
)

var (
	sessionsLimit int
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List and manage sessions",
}

var sessionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := rpcPing(cmd.Context()); err != nil {
			return err
		}
		if sessionsLimit <= 0 {
			sessionsLimit = 100
		}
		var out protocol.SessionListResult
		if err := rpcCall(cmd.Context(), protocol.MethodSessionList, protocol.SessionListParams{
			ThreadID: detachedThreadID,
			Limit:    sessionsLimit,
			Offset:   0,
		}, &out); err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "SESSION\tMODE\tTEAM\tRUN\tRUNNING\tPAUSED\tUPDATED")
		for _, s := range out.Sessions {
			fmt.Fprintf(
				w,
				"%s\t%s\t%s\t%s\t%d\t%d\t%s\n",
				strings.TrimSpace(s.SessionID),
				fallback(strings.TrimSpace(s.Mode), "standalone"),
				blankDash(strings.TrimSpace(s.TeamID)),
				blankDash(strings.TrimSpace(s.CurrentRunID)),
				s.RunningAgents,
				s.PausedAgents,
				blankDash(strings.TrimSpace(s.UpdatedAt)),
			)
		}
		return w.Flush()
	},
}

var sessionsAttachCmd = &cobra.Command{
	Use:   "attach <session-id>",
	Short: "Attach to a session coordinator",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCoordinatorFn(cmd, strings.TrimSpace(args[0]))
	},
}

var sessionsPauseCmd = &cobra.Command{
	Use:   "pause <session-id>",
	Short: "Pause all runs in a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := strings.TrimSpace(args[0])
		var out protocol.SessionPauseResult
		if err := rpcCall(cmd.Context(), protocol.MethodSessionPause, protocol.SessionPauseParams{
			ThreadID:  protocol.ThreadID(sessionID),
			SessionID: sessionID,
		}, &out); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Paused session %s (%d runs)\n", sessionID, len(out.AffectedRunIDs))
		return nil
	},
}

var sessionsResumeCmd = &cobra.Command{
	Use:   "resume <session-id>",
	Short: "Resume all runs in a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := strings.TrimSpace(args[0])
		var out protocol.SessionResumeResult
		if err := rpcCall(cmd.Context(), protocol.MethodSessionResume, protocol.SessionResumeParams{
			ThreadID:  protocol.ThreadID(sessionID),
			SessionID: sessionID,
		}, &out); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Resumed session %s (%d runs)\n", sessionID, len(out.AffectedRunIDs))
		return nil
	},
}

var sessionsStopCmd = &cobra.Command{
	Use:   "stop <session-id>",
	Short: "Stop all runs in a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := strings.TrimSpace(args[0])
		var out protocol.SessionStopResult
		if err := rpcCall(cmd.Context(), protocol.MethodSessionStop, protocol.SessionStopParams{
			ThreadID:  protocol.ThreadID(sessionID),
			SessionID: sessionID,
		}, &out); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Stopped session %s (%d runs)\n", sessionID, len(out.AffectedRunIDs))
		return nil
	},
}

var sessionsDeleteCmd = &cobra.Command{
	Use:   "delete <session-id>",
	Short: "Delete a session and its persisted history",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := strings.TrimSpace(args[0])
		var out protocol.SessionDeleteResult
		if err := rpcCall(cmd.Context(), protocol.MethodSessionDelete, protocol.SessionDeleteParams{
			SessionID: sessionID,
		}, &out); err != nil {
			return err
		}
		projectCtx, err := loadProjectContext()
		if err == nil && projectCtx.Exists && strings.TrimSpace(projectCtx.State.ActiveSessionID) == sessionID {
			_ = updateProjectActiveSession("", "", "", "sessions delete")
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted session %s\n", sessionID)
		return nil
	},
}

func blankDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return strings.TrimSpace(v)
}

func fallback(v, def string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	return v
}

func init() {
	sessionsListCmd.Flags().IntVar(&sessionsLimit, "limit", 100, "maximum sessions to show")

	sessionsCmd.AddCommand(sessionsListCmd)
	sessionsCmd.AddCommand(sessionsAttachCmd)
	sessionsCmd.AddCommand(sessionsPauseCmd)
	sessionsCmd.AddCommand(sessionsResumeCmd)
	sessionsCmd.AddCommand(sessionsStopCmd)
	sessionsCmd.AddCommand(sessionsDeleteCmd)
	rootCmd.AddCommand(sessionsCmd)
}
