package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	taskdomain "github.com/tinoosan/agen8/internal/services/task/domain"
)

type testMemberLoader struct {
	members map[member.ID]member.Record
}

func (s testMemberLoader) GetMember(_ context.Context, id member.ID) (member.Record, error) {
	record, ok := s.members[id]
	if !ok {
		return member.Record{}, member.ErrNotFound
	}
	return record, nil
}

func TestMergeTaskMetadataPreservesExistingValues(t *testing.T) {
	merged := mergeTaskMetadata(map[string]any{
		"missionRef": "mission-1",
		"owner":      "backend",
	}, map[string]any{
		"commit": "abc123",
		"owner":  "reviewed",
	})

	if merged["missionRef"] != "mission-1" {
		t.Fatalf("missionRef=%v", merged["missionRef"])
	}
	if merged["commit"] != "abc123" {
		t.Fatalf("commit=%v", merged["commit"])
	}
	if merged["owner"] != "reviewed" {
		t.Fatalf("owner=%v", merged["owner"])
	}
}

func TestMergeTaskMetadataCopiesExistingWhenNoUpdates(t *testing.T) {
	existing := map[string]any{"missionRef": "mission-1"}
	merged := mergeTaskMetadata(existing, nil)
	merged["missionRef"] = "changed"

	if existing["missionRef"] != "mission-1" {
		t.Fatalf("existing map was mutated: %+v", existing)
	}
}

func TestReviewMetadataPersistsReasonSummaryAndNote(t *testing.T) {
	svc := &Service{
		clock: taskdomain.FixedClock{T: time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)},
		members: testMemberLoader{members: map[member.ID]member.Record{
			"coord-1": {
				ID:             "coord-1",
				ProjectID:      "space-1",
				MemberType:     member.TypeCoordinator,
				DisplayName:    "QA Reviewer",
				LifecycleState: member.LifecycleActive,
			},
		}},
	}

	meta := svc.reviewMetadata(context.Background(), caller.Caller{MemberID: "coord-1", ProjectID: types.ProjectID("space-1")}, "space-1", "approve", "retry details", "quick pass", "deep verification")
	if got := meta["reviewReason"]; got != "retry details" {
		t.Fatalf("reviewReason=%v want retry details", got)
	}
	if got := meta["reviewSummary"]; got != "quick pass" {
		t.Fatalf("reviewSummary=%v want quick pass", got)
	}
	if got := meta["reviewNote"]; got != "deep verification" {
		t.Fatalf("reviewNote=%v want deep verification", got)
	}
	// reviewFeedback (the card-era duplicate) is no longer written.
	if _, ok := meta["reviewFeedback"]; ok {
		t.Fatalf("reviewFeedback should no longer be written: %+v", meta)
	}
}

type fakeTaskRepository struct {
	tasks          map[string]taskdomain.Task
	updatedTask    taskdomain.Task
	listTasksErr   error
	countTasksErr  error
	updateTaskErr  error
	getTaskErr     error
	getTaskErrID   string
	getTaskErrText string
}

func (r *fakeTaskRepository) GetTask(_ context.Context, taskID taskdomain.TaskID) (taskdomain.Task, error) {
	if r.getTaskErr != nil {
		return taskdomain.Task{}, r.getTaskErr
	}
	if r.getTaskErrID != "" && string(taskID) == r.getTaskErrID {
		return taskdomain.Task{}, errors.New(r.getTaskErrText)
	}
	task, ok := r.tasks[string(taskID)]
	if !ok {
		return taskdomain.Task{}, fmt.Errorf("task %s not found", taskID)
	}
	return task, nil
}

func (r *fakeTaskRepository) ListTasks(_ context.Context, _ taskdomain.TaskFilter) ([]taskdomain.Task, error) {
	if r.listTasksErr != nil {
		return nil, r.listTasksErr
	}
	return nil, nil
}

func (r *fakeTaskRepository) CreateTask(_ context.Context, task taskdomain.Task) error {
	if r.tasks == nil {
		r.tasks = map[string]taskdomain.Task{}
	}
	r.tasks[string(task.ID)] = task
	return nil
}

func (r *fakeTaskRepository) CountTasks(_ context.Context, _ taskdomain.TaskFilter) (int, error) {
	if r.countTasksErr != nil {
		return 0, r.countTasksErr
	}
	return 0, nil
}

func (r *fakeTaskRepository) UpdateTask(_ context.Context, task taskdomain.Task) error {
	if r.updateTaskErr != nil {
		return r.updateTaskErr
	}
	r.updatedTask = task
	if r.tasks != nil {
		r.tasks[string(task.ID)] = task
	}
	return nil
}

type fakeKeyResultMissionReader struct {
	missions map[string]string
	err      error
}

func (m fakeKeyResultMissionReader) KeyResultMission(_ context.Context, keyResultID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if missionID, ok := m.missions[keyResultID]; ok {
		return missionID, nil
	}
	return "", fmt.Errorf("key result %s not found", keyResultID)
}

type fakeCallerResolver struct {
	Caller caller.Caller
}

func (f fakeCallerResolver) ResolveCaller(context.Context) (caller.Caller, error) {
	return f.Caller, nil
}

func strPtr(value string) *string {
	return &value
}

func TestUpdateRejectsMismatchedKeyResultAndMetadataMissionRef(t *testing.T) {
	repo := &fakeTaskRepository{tasks: map[string]taskdomain.Task{
		"task-1": {
			ID:           "task-1",
			ProjectID:    "space-1",
			KeyResultRef: "kr-1",
			Metadata:     map[string]any{"missionRef": "mission-1"},
		},
	}}
	svc := &Service{
		clock:  taskdomain.FixedClock{T: time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)},
		logger: slog.Default(),
		tasks:  repo,
		caller: fakeCallerResolver{Caller: caller.Caller{MemberID: "member-1"}},
		members: testMemberLoader{members: map[member.ID]member.Record{
			"member-1": {
				ID:             "member-1",
				ProjectID:      "space-1",
				MemberType:     member.TypeCoordinator,
				LifecycleState: member.LifecycleActive,
			},
		}},
		missions: fakeKeyResultMissionReader{missions: map[string]string{
			"kr-1": "mission-1",
			"kr-2": "mission-2",
		}},
	}

	_, err := svc.Update(context.Background(), UpdateTaskParams{
		TaskID:       "task-1",
		KeyResultRef: strPtr("kr-2"),
	})
	if err == nil {
		t.Fatal("expected mission mismatch error")
	}
	if repo.updatedTask.ID != "" {
		t.Fatalf("update should not persist when inconsistent: %+v", repo.updatedTask)
	}
	if err == nil || !strings.Contains(err.Error(), "metadata provided mission-1") {
		t.Fatalf("error=%v", err)
	}
}

func TestUpdateNormalizesMissionMetadataForKeyResultRef(t *testing.T) {
	repo := &fakeTaskRepository{tasks: map[string]taskdomain.Task{
		"task-1": {
			ID:           "task-1",
			ProjectID:    "space-1",
			KeyResultRef: "kr-1",
			Metadata:     map[string]any{"missionRef": "mission-1", "owner": "owner-a"},
			Status:       taskdomain.TaskStatusPending,
		},
	}}
	svc := &Service{
		clock:  taskdomain.FixedClock{T: time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)},
		logger: slog.Default(),
		tasks:  repo,
		caller: fakeCallerResolver{Caller: caller.Caller{MemberID: "member-1"}},
		members: testMemberLoader{members: map[member.ID]member.Record{
			"member-1": {
				ID:             "member-1",
				ProjectID:      "space-1",
				MemberType:     member.TypeCoordinator,
				LifecycleState: member.LifecycleActive,
			},
		}},
		missions: fakeKeyResultMissionReader{missions: map[string]string{
			"kr-1": "mission-1",
			"kr-2": "mission-2",
		}},
	}

	updated, err := svc.Update(context.Background(), UpdateTaskParams{
		TaskID:       "task-1",
		KeyResultRef: strPtr("kr-2"),
		Metadata: map[string]any{
			"owner": "owner-b",
		},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.KeyResultRef != "kr-2" {
		t.Fatalf("keyResultRef=%q want kr-2", updated.KeyResultRef)
	}
	if got, ok := updated.Metadata["missionRef"]; !ok || got != "mission-2" {
		t.Fatalf("missionRef=%v want mission-2", got)
	}
	if got := updated.Metadata["owner"]; got != "owner-b" {
		t.Fatalf("owner=%v want owner-b", got)
	}
	if saved := repo.updatedTask.Metadata; saved["missionRef"] != "mission-2" || saved["owner"] != "owner-b" {
		t.Fatalf("updated task metadata=%+v", saved)
	}
}

func TestUpdateRejectsMetadataMissionRefDriftForExistingKeyResult(t *testing.T) {
	repo := &fakeTaskRepository{tasks: map[string]taskdomain.Task{
		"task-1": {
			ID:           "task-1",
			ProjectID:    "space-1",
			KeyResultRef: "kr-1",
			Metadata:     map[string]any{"missionRef": "mission-1"},
		},
	}}
	svc := &Service{
		clock:  taskdomain.FixedClock{T: time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)},
		logger: slog.Default(),
		tasks:  repo,
		caller: fakeCallerResolver{Caller: caller.Caller{MemberID: "member-1"}},
		members: testMemberLoader{members: map[member.ID]member.Record{
			"member-1": {
				ID:             "member-1",
				ProjectID:      "space-1",
				MemberType:     member.TypeCoordinator,
				LifecycleState: member.LifecycleActive,
			},
		}},
		missions: fakeKeyResultMissionReader{missions: map[string]string{
			"kr-1": "mission-1",
			"kr-2": "mission-2",
		}},
	}

	_, err := svc.Update(context.Background(), UpdateTaskParams{
		TaskID:   "task-1",
		Metadata: map[string]any{"missionRef": "mission-2"},
	})
	if err == nil {
		t.Fatal("expected mission mismatch error")
	}
	if !strings.Contains(err.Error(), "metadata provided mission-2") {
		t.Fatalf("error=%v", err)
	}
}

func attachTestService(repo *fakeTaskRepository, callerID string) *Service {
	return &Service{
		clock:  taskdomain.FixedClock{T: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)},
		logger: slog.Default(),
		tasks:  repo,
		caller: fakeCallerResolver{Caller: caller.Caller{MemberID: callerID}},
		members: testMemberLoader{members: map[member.ID]member.Record{
			"worker-1": {
				ID:             "worker-1",
				ProjectID:      "space-1",
				MemberType:     member.TypeWorker,
				LifecycleState: member.LifecycleActive,
			},
			"outsider-1": {
				ID:             "outsider-1",
				ProjectID:      "space-other",
				MemberType:     member.TypeWorker,
				LifecycleState: member.LifecycleActive,
			},
		}},
	}
}

func TestAttachArtifactAppendsWithoutClobberingExistingArtifacts(t *testing.T) {
	repo := &fakeTaskRepository{tasks: map[string]taskdomain.Task{
		"task-1": {
			ID:        "task-1",
			ProjectID: "space-1",
			Status:    taskdomain.TaskStatusActive,
			Artifacts: []string{"commit:abc123", "file:/project/notes.md"},
		},
	}}
	svc := attachTestService(repo, "worker-1")

	next, err := svc.AttachArtifact(context.Background(), "task-1", "file:/project/.agen8/attachments/task-1/shot.png")
	if err != nil {
		t.Fatalf("AttachArtifact: %v", err)
	}
	want := []string{"commit:abc123", "file:/project/notes.md", "file:/project/.agen8/attachments/task-1/shot.png"}
	if len(next.Artifacts) != len(want) {
		t.Fatalf("artifacts=%v want %v", next.Artifacts, want)
	}
	for i := range want {
		if next.Artifacts[i] != want[i] {
			t.Fatalf("artifacts[%d]=%q want %q", i, next.Artifacts[i], want[i])
		}
	}
	if len(repo.updatedTask.Artifacts) != 3 {
		t.Fatalf("persisted artifacts=%v want 3 entries", repo.updatedTask.Artifacts)
	}
}

func TestAttachArtifactAllowsAnyActiveProjectMember(t *testing.T) {
	// A plain worker (not coordinator, not assignee) may attach — reviewers
	// and collaborators add evidence to tasks they do not own.
	repo := &fakeTaskRepository{tasks: map[string]taskdomain.Task{
		"task-1": {ID: "task-1", ProjectID: "space-1", AssignedTo: "someone-else", Status: taskdomain.TaskStatusInReview},
	}}
	svc := attachTestService(repo, "worker-1")
	if _, err := svc.AttachArtifact(context.Background(), "task-1", "file:/project/a.png"); err != nil {
		t.Fatalf("worker attach should be allowed: %v", err)
	}
}

func TestAttachArtifactRejectsMemberOutsideProject(t *testing.T) {
	repo := &fakeTaskRepository{tasks: map[string]taskdomain.Task{
		"task-1": {ID: "task-1", ProjectID: "space-1", Status: taskdomain.TaskStatusActive},
	}}
	svc := attachTestService(repo, "outsider-1")
	_, err := svc.AttachArtifact(context.Background(), "task-1", "file:/project/a.png")
	if err == nil || !strings.Contains(err.Error(), "not in project") {
		t.Fatalf("expected project membership rejection, got %v", err)
	}
	if repo.updatedTask.ID != "" {
		t.Fatalf("attach persisted despite rejection: %+v", repo.updatedTask)
	}
}

func TestAttachArtifactRejectsCanceledTask(t *testing.T) {
	repo := &fakeTaskRepository{tasks: map[string]taskdomain.Task{
		"task-1": {ID: "task-1", ProjectID: "space-1", Status: taskdomain.TaskStatusCanceled},
	}}
	svc := attachTestService(repo, "worker-1")
	_, err := svc.AttachArtifact(context.Background(), "task-1", "file:/project/a.png")
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("expected canceled rejection, got %v", err)
	}
	if repo.updatedTask.ID != "" {
		t.Fatalf("attach persisted despite rejection: %+v", repo.updatedTask)
	}
}

func TestAttachArtifactRejectsNonexistentTask(t *testing.T) {
	repo := &fakeTaskRepository{tasks: map[string]taskdomain.Task{}}
	svc := attachTestService(repo, "worker-1")
	_, err := svc.AttachArtifact(context.Background(), "task-missing", "file:/project/a.png")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found rejection, got %v", err)
	}
	if repo.updatedTask.ID != "" {
		t.Fatalf("attach persisted despite missing task: %+v", repo.updatedTask)
	}
}
