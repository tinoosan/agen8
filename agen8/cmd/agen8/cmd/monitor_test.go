package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/tinoosan/agen8/internal/tui"
	"github.com/tinoosan/agen8/pkg/config"
)

func TestMonitorCommand_RetriesOnSwitchError(t *testing.T) {
	origRunMonitor := runMonitorFn
	origRunTeamMonitor := runTeamMonitorFn
	origRunDetachedMonitor := runDetachedMonitorFn
	t.Cleanup(func() {
		runMonitorFn = origRunMonitor
		runTeamMonitorFn = origRunTeamMonitor
		runDetachedMonitorFn = origRunDetachedMonitor
	})

	monitorAgentID = "run-a"
	monitorTeamID = ""
	t.Cleanup(func() { monitorAgentID = "" })
	t.Cleanup(func() { monitorTeamID = "" })

	calls := 0
	runMonitorFn = func(_ context.Context, _ config.Config, runID string) error {
		calls++
		switch calls {
		case 1:
			if runID != "run-a" {
				t.Fatalf("first call runID=%q, want %q", runID, "run-a")
			}
			return &tui.MonitorSwitchRunError{RunID: "run-b"}
		case 2:
			if runID != "run-b" {
				t.Fatalf("second call runID=%q, want %q", runID, "run-b")
			}
			return nil
		default:
			t.Fatalf("unexpected extra runMonitor call: %d", calls)
		}
		return nil
	}

	if err := monitorCmd.RunE(monitorCmd, nil); err != nil {
		t.Fatalf("monitor command returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want 2", calls)
	}
}

func TestMonitorCommand_ReturnsNonSwitchError(t *testing.T) {
	origRunMonitor := runMonitorFn
	origRunTeamMonitor := runTeamMonitorFn
	origRunDetachedMonitor := runDetachedMonitorFn
	t.Cleanup(func() {
		runMonitorFn = origRunMonitor
		runTeamMonitorFn = origRunTeamMonitor
		runDetachedMonitorFn = origRunDetachedMonitor
	})

	monitorAgentID = "run-a"
	monitorTeamID = ""
	t.Cleanup(func() { monitorAgentID = "" })
	t.Cleanup(func() { monitorTeamID = "" })

	wantErr := errors.New("boom")
	runMonitorFn = func(_ context.Context, _ config.Config, _ string) error {
		return wantErr
	}

	err := monitorCmd.RunE(monitorCmd, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestMonitorCommand_NoFlagsStartsDetached(t *testing.T) {
	origRunMonitor := runMonitorFn
	origRunTeamMonitor := runTeamMonitorFn
	origRunDetachedMonitor := runDetachedMonitorFn
	t.Cleanup(func() {
		runMonitorFn = origRunMonitor
		runTeamMonitorFn = origRunTeamMonitor
		runDetachedMonitorFn = origRunDetachedMonitor
	})

	monitorAgentID = ""
	monitorTeamID = ""
	t.Cleanup(func() {
		monitorAgentID = ""
		monitorTeamID = ""
	})

	calledDetached := false
	runDetachedMonitorFn = func(_ context.Context, _ config.Config) error {
		calledDetached = true
		return nil
	}
	runMonitorFn = func(_ context.Context, _ config.Config, _ string) error {
		t.Fatalf("runMonitorFn should not be called in detached mode")
		return nil
	}
	runTeamMonitorFn = func(_ context.Context, _ config.Config, _ string) error {
		t.Fatalf("runTeamMonitorFn should not be called in detached mode")
		return nil
	}

	if err := monitorCmd.RunE(monitorCmd, nil); err != nil {
		t.Fatalf("monitor command returned error: %v", err)
	}
	if !calledDetached {
		t.Fatalf("expected detached monitor to be started")
	}
}
