package webhook

import (
	"errors"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestBuildStandaloneTask(t *testing.T) {
	run := types.Run{SessionID: "s1", RunID: "r1"}
	payload := []byte(`{"goal":"do something","priority":1}`)
	task, err := BuildStandaloneTask(payload, run)
	if err != nil {
		t.Fatalf("BuildStandaloneTask: %v", err)
	}
	if task.Goal != "do something" {
		t.Errorf("Goal = %q, want do something", task.Goal)
	}
	if task.Priority != 1 {
		t.Errorf("Priority = %d, want 1", task.Priority)
	}
	if task.SessionID != "s1" || task.RunID != "r1" {
		t.Errorf("SessionID=%q RunID=%q, want s1 r1", task.SessionID, task.RunID)
	}
	if task.TaskID == "" {
		t.Error("TaskID should be auto-generated")
	}
}

func TestBuildStandaloneTask_GoalRequired(t *testing.T) {
	payload := []byte(`{"goal":""}`)
	_, err := BuildStandaloneTask(payload, types.Run{})
	if err == nil {
		t.Fatal("expected error for empty goal")
	}
	if !errors.Is(err, errGoalRequired) {
		t.Errorf("err = %v, want errGoalRequired", err)
	}
}

func TestBuildTeamTask(t *testing.T) {
	run := types.Run{SessionID: "s1", RunID: "r1"}
	payload := []byte(`{"goal":"research","assignedRole":"ceo"}`)
	roleSet := map[string]struct{}{"ceo": {}, "cto": {}}
	task, err := BuildTeamTask(payload, "team-1", "ceo", run, roleSet)
	if err != nil {
		t.Fatalf("BuildTeamTask: %v", err)
	}
	if task.Goal != "research" {
		t.Errorf("Goal = %q, want research", task.Goal)
	}
	if task.AssignedRole != "ceo" {
		t.Errorf("AssignedRole = %q, want ceo", task.AssignedRole)
	}
	if task.TeamID != "team-1" {
		t.Errorf("TeamID = %q, want team-1", task.TeamID)
	}
	if task.Metadata["source"] != "webhook" {
		t.Errorf("Metadata[source] = %v", task.Metadata["source"])
	}
}

func TestBuildTeamTask_InvalidRole(t *testing.T) {
	payload := []byte(`{"goal":"x","assignedRole":"invalid"}`)
	roleSet := map[string]struct{}{"ceo": {}}
	_, err := BuildTeamTask(payload, "team-1", "ceo", types.Run{}, roleSet)
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
	if !errors.Is(err, errInvalidRole) {
		t.Errorf("err = %v, want errInvalidRole", err)
	}
}
