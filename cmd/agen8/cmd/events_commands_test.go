package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/app"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestResolveTargetRunIDs_UsesProjectSessionsByDefault(t *testing.T) {
	origProjectSessions := logsListProjectSessionsFn
	origSessionRuns := logsListSessionRunIDsFn
	origProjectTeamRuns := logsListProjectTeamRunIDsFn
	origProjectTeams := logsListProjectTeamsFn
	origWorkDir := workDir
	base := t.TempDir()
	workDir = base
	t.Cleanup(func() {
		logsListProjectSessionsFn = origProjectSessions
		logsListSessionRunIDsFn = origSessionRuns
		logsListProjectTeamRunIDsFn = origProjectTeamRuns
		logsListProjectTeamsFn = origProjectTeams
		workDir = origWorkDir
	})

	if _, err := app.InitProject(base, app.ProjectConfig{}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}

	logsListProjectTeamsFn = func(ctx context.Context, projectRoot string) ([]protocol.ProjectTeamSummary, error) {
		return []protocol.ProjectTeamSummary{
			{PrimarySessionID: "sess-b"},
			{PrimarySessionID: "sess-a"},
		}, nil
	}
	logsListProjectTeamRunIDsFn = func(ctx context.Context, projectRoot string, teamID string) ([]string, error) {
		t.Fatalf("unexpected team scope lookup for team %q", teamID)
		return nil, nil
	}
	logsListSessionRunIDsFn = func(ctx context.Context, sessionID string) ([]string, error) {
		switch sessionID {
		case "sess-a":
			return []string{"run-2", "run-1"}, nil
		case "sess-b":
			return []string{"run-3"}, nil
		default:
			return nil, nil
		}
	}

	got, err := resolveTargetRunIDs(context.Background(), "", "", "", "")
	if err != nil {
		t.Fatalf("resolveTargetRunIDs: %v", err)
	}
	want := []string{"run-1", "run-2", "run-3"}
	if len(got) != len(want) {
		t.Fatalf("run count=%d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("runIDs[%d]=%q want %q (%v)", i, got[i], want[i], got)
		}
	}
}

func TestResolveRunRoleLabels_UsesProjectSessions(t *testing.T) {
	origProjectSessions := logsListProjectSessionsFn
	origSessionAgents := logsListSessionAgentsFn
	origWorkDir := workDir
	base := t.TempDir()
	workDir = base
	t.Cleanup(func() {
		logsListProjectSessionsFn = origProjectSessions
		logsListSessionAgentsFn = origSessionAgents
		workDir = origWorkDir
	})

	if _, err := app.InitProject(base, app.ProjectConfig{}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}

	logsListProjectSessionsFn = func(ctx context.Context, projectRoot string) ([]string, error) {
		return []string{"sess-b", "sess-a"}, nil
	}
	logsListSessionAgentsFn = func(ctx context.Context, sessionID string) ([]protocol.AgentListItem, error) {
		switch sessionID {
		case "sess-a":
			return []protocol.AgentListItem{
				{RunID: "run-1", Role: "researcher", TeamID: "nirvana"},
				{RunID: "run-2", Role: "reviewer", TeamID: "nirvana"},
			}, nil
		case "sess-b":
			return []protocol.AgentListItem{
				{RunID: "run-3", Role: "coordinator", TeamID: "other"},
			}, nil
		default:
			return nil, nil
		}
	}

	got, err := resolveRunRoleLabels(context.Background(), []string{"run-3", "run-2", "run-1"}, "", "")
	if err != nil {
		t.Fatalf("resolveRunRoleLabels: %v", err)
	}
	if got["run-1"] != "researcher" || got["run-2"] != "reviewer" || got["run-3"] != "coordinator" {
		t.Fatalf("roles=%v", got)
	}
}

func TestResolveRunRoleLabels_FiltersByTeamID(t *testing.T) {
	origSessionAgents := logsListSessionAgentsFn
	t.Cleanup(func() {
		logsListSessionAgentsFn = origSessionAgents
	})

	logsListSessionAgentsFn = func(ctx context.Context, sessionID string) ([]protocol.AgentListItem, error) {
		if sessionID != "sess-1" {
			t.Fatalf("sessionID=%q want sess-1", sessionID)
		}
		return []protocol.AgentListItem{
			{RunID: "run-1", Role: "researcher", TeamID: "nirvana"},
			{RunID: "run-2", Role: "reviewer", TeamID: "other"},
		}, nil
	}

	got, err := resolveRunRoleLabels(context.Background(), []string{"run-1", "run-2"}, "sess-1", "nirvana")
	if err != nil {
		t.Fatalf("resolveRunRoleLabels: %v", err)
	}
	if got["run-1"] != "researcher" {
		t.Fatalf("role for run-1=%q", got["run-1"])
	}
	if _, ok := got["run-2"]; ok {
		t.Fatalf("unexpected role for run-2: %v", got)
	}
}

func TestResolveTargetRunIDs_FallsBackToSessionScope(t *testing.T) {
	origProjectSessions := logsListProjectSessionsFn
	origSessionRuns := logsListSessionRunIDsFn
	origProjectTeamRuns := logsListProjectTeamRunIDsFn
	t.Cleanup(func() {
		logsListProjectSessionsFn = origProjectSessions
		logsListSessionRunIDsFn = origSessionRuns
		logsListProjectTeamRunIDsFn = origProjectTeamRuns
	})

	logsListProjectSessionsFn = func(ctx context.Context, projectRoot string) ([]string, error) {
		return nil, nil
	}
	logsListProjectTeamRunIDsFn = func(ctx context.Context, projectRoot string, teamID string) ([]string, error) {
		t.Fatalf("unexpected team scope lookup for team %q", teamID)
		return nil, nil
	}
	logsListSessionRunIDsFn = func(ctx context.Context, sessionID string) ([]string, error) {
		if sessionID != "sess-1" {
			t.Fatalf("sessionID=%q want sess-1", sessionID)
		}
		return []string{"run-1"}, nil
	}

	got, err := resolveTargetRunIDs(context.Background(), "", "sess-1", "", "")
	if err != nil {
		t.Fatalf("resolveTargetRunIDs: %v", err)
	}
	if len(got) != 1 || got[0] != "run-1" {
		t.Fatalf("runIDs=%v", got)
	}
}

func TestResolveTargetRunIDs_UsesProjectTeamScope(t *testing.T) {
	origProjectSessions := logsListProjectSessionsFn
	origSessionRuns := logsListSessionRunIDsFn
	origProjectTeamRuns := logsListProjectTeamRunIDsFn
	origProjectTeam := logsGetProjectTeamFn
	origWorkDir := workDir
	base := t.TempDir()
	workDir = base
	t.Cleanup(func() {
		logsListProjectSessionsFn = origProjectSessions
		logsListSessionRunIDsFn = origSessionRuns
		logsListProjectTeamRunIDsFn = origProjectTeamRuns
		logsGetProjectTeamFn = origProjectTeam
		workDir = origWorkDir
	})

	if _, err := app.InitProject(base, app.ProjectConfig{}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}

	logsListProjectSessionsFn = func(ctx context.Context, projectRoot string) ([]string, error) {
		t.Fatalf("unexpected project session lookup for %q", projectRoot)
		return nil, nil
	}
	logsGetProjectTeamFn = func(ctx context.Context, projectRoot, teamID string) (protocol.ProjectTeamSummary, error) {
		if teamID != "research" {
			t.Fatalf("teamID=%q want research", teamID)
		}
		return protocol.ProjectTeamSummary{PrimarySessionID: "sess-research"}, nil
	}
	logsListSessionRunIDsFn = func(ctx context.Context, sessionID string) ([]string, error) {
		if sessionID != "sess-research" {
			t.Fatalf("sessionID=%q want sess-research", sessionID)
		}
		return []string{"run-2", "run-1"}, nil
	}

	got, err := resolveTargetRunIDs(context.Background(), "", "", "", "research")
	if err != nil {
		t.Fatalf("resolveTargetRunIDs: %v", err)
	}
	want := []string{"run-1", "run-2"}
	if len(got) != len(want) {
		t.Fatalf("run count=%d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("runIDs[%d]=%q want %q (%v)", i, got[i], want[i], got)
		}
	}
}

func TestLogsHelp_HidesInternalScopeFlags(t *testing.T) {
	output := new(strings.Builder)
	logsCmd.SetOut(output)
	logsCmd.SetErr(output)
	t.Cleanup(func() {
		logsCmd.SetOut(nil)
		logsCmd.SetErr(nil)
	})

	err := logsCmd.Help()
	if err != nil {
		t.Fatalf("logsCmd.Execute: %v", err)
	}

	helpText := output.String()
	for _, hiddenFlag := range []string{"--run-id", "--session-id", "--agent"} {
		if strings.Contains(helpText, hiddenFlag) {
			t.Fatalf("help unexpectedly contains %s\n%s", hiddenFlag, helpText)
		}
	}
	if !strings.Contains(helpText, "--team-id") {
		t.Fatalf("help missing --team-id\n%s", helpText)
	}
}

func TestFormatEventLine_IncludesRunIDAndRole(t *testing.T) {
	ev := types.EventRecord{
		Timestamp: time.Date(2026, time.March, 6, 12, 30, 0, 0, time.UTC),
		RunID:     "nirvana-run-1",
		Type:      "task.started",
		Message:   "working",
	}

	got := formatEventLine(ev, "researcher")
	if !strings.Contains(got, "nirvana-run-1") {
		t.Fatalf("missing run id in %q", got)
	}
	if !strings.Contains(got, "researcher") {
		t.Fatalf("missing role in %q", got)
	}
	if !strings.Contains(got, "task.started") {
		t.Fatalf("missing type in %q", got)
	}
}
