package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/pkg/protocol"
)

type coordinatorSessionState struct {
	sessionID       string
	runID           string
	teamID          string
	mode            string
	coordinatorRole string
}

func runCoordinatorShell(cmd *cobra.Command, sessionID string, runID string, teamID string) error {
	state, err := resolveCoordinatorState(cmd, sessionID, runID, teamID)
	if err != nil {
		return err
	}
	if err := updateProjectActiveSession(state.sessionID, state.teamID, state.runID, "coordinator"); err != nil {
		return err
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	var activityCursor int64
	fmt.Fprintf(cmd.OutOrStdout(), "Coordinator session %s (mode=%s, run=%s)\n", state.sessionID, state.mode, state.runID)
	fmt.Fprintln(cmd.OutOrStdout(), "Commands: /new /attach <session-id> /pause /resume /stop /reviewer /reconnect /help /quit")
	for {
		if next, err := printActivityBatch(cmd, state.runID, activityCursor, 20); err == nil && next > 0 {
			activityCursor = next
		}
		if latest, err := rpcLatestSeq(cmd, state.runID); err == nil {
			activityCursor = latest
		}

		fmt.Fprint(cmd.OutOrStdout(), "coordinator> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			nextState, shouldExit, err := handleCoordinatorCommand(cmd, state, line)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "error: %v\n", err)
				continue
			}
			state = nextState
			if shouldExit {
				return nil
			}
			continue
		}

		if err := submitCoordinatorGoal(cmd, state, line); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "error: %v\n", err)
			continue
		}
		fmt.Fprintln(cmd.OutOrStdout(), "queued")
	}
}

func handleCoordinatorCommand(cmd *cobra.Command, state coordinatorSessionState, line string) (coordinatorSessionState, bool, error) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return state, false, nil
	}
	switch parts[0] {
	case "/quit":
		return state, true, nil
	case "/help":
		fmt.Fprintln(cmd.OutOrStdout(), "Commands: /new /attach <session-id> /pause /resume /stop /reviewer /reconnect /help /quit")
		return state, false, nil
	case "/reviewer":
		if strings.TrimSpace(state.coordinatorRole) == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "reviewer: auto-managed by runtime")
			return state, false, nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "coordinator role: %s\n", state.coordinatorRole)
		return state, false, nil
	case "/new":
		return state, false, runNewSessionFlow(cmd, true)
	case "/attach":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return state, false, fmt.Errorf("usage: /attach <session-id>")
		}
		next, err := resolveCoordinatorState(cmd, strings.TrimSpace(parts[1]), "", "")
		if err != nil {
			return state, false, err
		}
		if err := updateProjectActiveSession(next.sessionID, next.teamID, next.runID, "attach"); err != nil {
			return state, false, err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "attached to %s (run=%s)\n", next.sessionID, next.runID)
		return next, false, nil
	case "/pause":
		return state, false, rpcSessionActionWithRecovery(cmd, state, protocol.MethodSessionPause)
	case "/resume":
		return state, false, rpcSessionActionWithRecovery(cmd, state, protocol.MethodSessionResume)
	case "/stop":
		return state, false, rpcSessionActionWithRecovery(cmd, state, protocol.MethodSessionStop)
	case "/reconnect":
		next, err := resolveCoordinatorState(cmd, state.sessionID, "", "")
		if err != nil {
			return state, false, err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "reconnected")
		return next, false, nil
	default:
		return state, false, fmt.Errorf("unknown command: %s", parts[0])
	}
}

func rpcSessionActionWithRecovery(cmd *cobra.Command, state coordinatorSessionState, method string) error {
	call := func() error {
		switch method {
		case protocol.MethodSessionPause:
			return rpcCall(cmd.Context(), method, protocol.SessionPauseParams{
				ThreadID:  protocol.ThreadID(state.sessionID),
				SessionID: state.sessionID,
			}, &protocol.SessionPauseResult{})
		case protocol.MethodSessionResume:
			return rpcCall(cmd.Context(), method, protocol.SessionResumeParams{
				ThreadID:  protocol.ThreadID(state.sessionID),
				SessionID: state.sessionID,
			}, &protocol.SessionResumeResult{})
		case protocol.MethodSessionStop:
			return rpcCall(cmd.Context(), method, protocol.SessionStopParams{
				ThreadID:  protocol.ThreadID(state.sessionID),
				SessionID: state.sessionID,
			}, &protocol.SessionStopResult{})
		default:
			return fmt.Errorf("unsupported session method %s", method)
		}
	}
	err := call()
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "thread not found") {
		return err
	}
	resolved, rerr := rpcResolveThread(cmd.Context(), state.sessionID, state.runID)
	if rerr != nil {
		return err
	}
	_ = updateProjectActiveSession(resolved.SessionID, resolved.TeamID, resolved.RunID, "reconnect")
	return call()
}

func submitCoordinatorGoal(cmd *cobra.Command, state coordinatorSessionState, goal string) error {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return fmt.Errorf("goal is required")
	}
	retries := 0
	for {
		var out protocol.TaskCreateResult
		err := rpcCall(cmd.Context(), protocol.MethodTaskCreate, protocol.TaskCreateParams{
			ThreadID:     protocol.ThreadID(state.sessionID),
			TeamID:       state.teamID,
			RunID:        state.runID,
			Goal:         goal,
			TaskKind:     "user_message",
			AssignedRole: state.coordinatorRole,
		}, &out)
		if err == nil {
			return nil
		}
		if !isRetryableLiveError(err) || retries >= 5 {
			return err
		}
		time.Sleep(time.Duration(300*(1<<retries)) * time.Millisecond)
		retries++
	}
}

func resolveCoordinatorState(cmd *cobra.Command, sessionID, runID, teamID string) (coordinatorSessionState, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return coordinatorSessionState{}, fmt.Errorf("session id is required")
	}
	item, err := rpcFindSession(cmd.Context(), sessionID)
	if err != nil {
		return coordinatorSessionState{}, err
	}
	state := coordinatorSessionState{
		sessionID: sessionID,
		teamID:    strings.TrimSpace(item.TeamID),
		mode:      fallback(item.Mode, "standalone"),
	}
	if strings.TrimSpace(teamID) != "" {
		state.teamID = strings.TrimSpace(teamID)
	}
	state.runID = strings.TrimSpace(runID)
	if state.runID == "" {
		resolvedRunID, resolvedTeamID, err := rpcResolveCoordinatorRun(cmd.Context(), sessionID)
		if err != nil {
			return coordinatorSessionState{}, err
		}
		state.runID = strings.TrimSpace(resolvedRunID)
		if strings.TrimSpace(resolvedTeamID) != "" {
			state.teamID = strings.TrimSpace(resolvedTeamID)
		}
	}
	if state.teamID != "" {
		var manifest protocol.TeamGetManifestResult
		if err := rpcCall(cmd.Context(), protocol.MethodTeamGetManifest, protocol.TeamGetManifestParams{
			ThreadID: protocol.ThreadID(sessionID),
			TeamID:   state.teamID,
		}, &manifest); err == nil {
			if strings.TrimSpace(manifest.CoordinatorRun) != "" {
				state.runID = strings.TrimSpace(manifest.CoordinatorRun)
			}
			state.coordinatorRole = strings.TrimSpace(manifest.CoordinatorRole)
		}
	}
	if state.runID == "" {
		return coordinatorSessionState{}, fmt.Errorf("no run found for session %s", sessionID)
	}
	return state, nil
}

func rpcLatestSeq(cmd *cobra.Command, runID string) (int64, error) {
	var latest protocol.EventsLatestSeqResult
	if err := rpcCall(cmd.Context(), protocol.MethodEventsLatestSeq, protocol.EventsLatestSeqParams{RunID: strings.TrimSpace(runID)}, &latest); err != nil {
		return 0, err
	}
	return latest.Seq, nil
}

func isRetryableLiveError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "connection refused"):
		return true
	case strings.Contains(msg, "broken pipe"):
		return true
	case strings.Contains(msg, "reset by peer"):
		return true
	case strings.Contains(msg, "i/o timeout"):
		return true
	case strings.Contains(msg, "timeout"):
		return true
	case strings.Contains(msg, "eof"):
		return true
	case strings.Contains(msg, "thread not found"):
		return true
	default:
		return false
	}
}
