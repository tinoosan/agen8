package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
)

var fixedTime = time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

func makeTask(status TaskStatus) Task {
	return Task{
		ID:          TaskID("task-1"),
		ProjectID:   types.ProjectID("project-1"),
		AssignedTo:  member.ID("member-fred"),
		Description: "do the thing",
		AcceptanceCriteria: []AcceptanceCriterion{
			{ID: "criterion-1", Text: "tests pass"},
			{ID: "criterion-2", Text: "docs updated"},
		},
		CreatedBy: "coordinator",
		Status:    status,
	}
}

func TestClaim_HappyPath(t *testing.T) {
	next, err := makeTask(TaskStatusPending).Claim(member.ID("member-fred"), fixedTime)
	if err != nil {
		t.Fatalf("Claim returned error: %v", err)
	}
	if next.Status != TaskStatusActive {
		t.Fatalf("status=%s want active", next.Status)
	}
	if next.ClaimedBy() != "member-fred" {
		t.Fatalf("claimedBy=%q want member-fred", next.ClaimedBy())
	}
	if next.StartedAt == nil || !next.StartedAt.Equal(fixedTime) {
		t.Fatalf("StartedAt=%v want %v", next.StartedAt, fixedTime)
	}
}

func TestClaim_RejectsWrongMember(t *testing.T) {
	_, err := makeTask(TaskStatusPending).Claim(member.ID("member-other"), fixedTime)
	if err == nil {
		t.Fatal("expected wrong member claim to fail")
	}
}

func TestClaim_RejectsEmptyMember(t *testing.T) {
	_, err := makeTask(TaskStatusPending).Claim(member.ID(" "), fixedTime)
	if err == nil {
		t.Fatal("expected empty member claim to fail")
	}
}

func TestClaim_RejectsNonPendingStatus(t *testing.T) {
	for _, status := range []TaskStatus{
		TaskStatusActive,
		TaskStatusBlocked,
		TaskStatusInReview,
		TaskStatusSucceeded,
		TaskStatusFailed,
		TaskStatusCanceled,
	} {
		_, err := makeTask(status).Claim(member.ID("member-fred"), fixedTime)
		if err == nil {
			t.Fatalf("expected error claiming from status %s", status)
		}
	}
}

func TestAssign(t *testing.T) {
	active := makeTask(TaskStatusActive)
	active.ClaimedByMemberID = member.ID("member-fred")
	active.ClaimedByMemberLabel = "Fred"
	next, err := active.Assign(member.ID("member-jane"), fixedTime)
	if err != nil {
		t.Fatalf("Assign returned error: %v", err)
	}
	if next.AssignedMemberID() != "member-jane" {
		t.Fatalf("assignedTo=%q want member-jane", next.AssignedMemberID())
	}
	if next.Status != TaskStatusPending {
		t.Fatalf("status=%s want pending", next.Status)
	}
	if next.ClaimedBy() != "" {
		t.Fatalf("claim should be cleared")
	}
	if next.ClaimedByMemberLabel != "" {
		t.Fatalf("claim label should be cleared, got %q", next.ClaimedByMemberLabel)
	}
}

func TestAssign_RejectsTerminalTask(t *testing.T) {
	terminal := makeTask(TaskStatusSucceeded)
	if _, err := terminal.Assign(member.ID("member-jane"), fixedTime); err == nil {
		t.Fatal("expected assign terminal to fail")
	}
}

func TestComplete_MovesActiveTaskToReview(t *testing.T) {
	active := makeTask(TaskStatusActive)
	active.ClaimedByMemberID = member.ID("member-fred")

	next, err := active.Complete("done", []string{"out.txt", "out.txt", " "}, fixedTime)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if next.Status != TaskStatusInReview {
		t.Fatalf("status=%s want in_review", next.Status)
	}
	if next.Summary != "done" {
		t.Fatalf("summary=%q want done", next.Summary)
	}
	if got := next.Artifacts; len(got) != 1 || got[0] != "out.txt" {
		t.Fatalf("artifacts=%v want [out.txt]", got)
	}
	if next.CompletedAt != nil {
		t.Fatal("CompletedAt should not be stamped before coordinator approval")
	}
}

func TestComplete_RejectsMissingSummaryOrClaim(t *testing.T) {
	active := makeTask(TaskStatusActive)
	if _, err := active.Complete("done", nil, fixedTime); err == nil {
		t.Fatal("expected unclaimed task completion to fail")
	}
	active.ClaimedByMemberID = member.ID("member-fred")
	if _, err := active.Complete(" ", nil, fixedTime); err == nil {
		t.Fatal("expected empty summary completion to fail")
	}
}

func TestComplete_RejectsNonActiveStatus(t *testing.T) {
	for _, status := range []TaskStatus{
		TaskStatusPending,
		TaskStatusBlocked,
		TaskStatusInReview,
		TaskStatusSucceeded,
	} {
		task := makeTask(status)
		task.ClaimedByMemberID = member.ID("member-fred")
		_, err := task.Complete("done", nil, fixedTime)
		if err == nil {
			t.Fatalf("expected error completing from status %s", status)
		}
	}
}

func TestReviewApprove_HappyPath(t *testing.T) {
	inReview := makeTask(TaskStatusInReview)
	inReview.ClaimedByMemberID = member.ID("member-fred")
	inReview.ClaimedByMemberLabel = "Fred"
	inReview.Summary = "done"

	next, err := inReview.ApproveReview([]CriterionReview{
		{ID: "criterion-1", Satisfied: true},
		{ID: "criterion-2", Satisfied: true},
	}, fixedTime)
	if err != nil {
		t.Fatalf("ApproveReview returned error: %v", err)
	}
	if next.Status != TaskStatusSucceeded {
		t.Fatalf("status=%s want succeeded", next.Status)
	}
	if next.ClaimedBy() != member.ID("member-fred") {
		t.Fatalf("claim should be retained on approval for attribution, got %q", next.ClaimedBy())
	}
	if next.ClaimedByMemberLabel != "Fred" {
		t.Fatalf("claim label should be retained on approval, got %q", next.ClaimedByMemberLabel)
	}
	if next.CompletedAt == nil {
		t.Fatal("CompletedAt should be stamped on approval")
	}
	for _, criterion := range next.AcceptanceCriteria {
		if !criterion.Satisfied {
			t.Fatalf("criterion %s should be satisfied", criterion.ID)
		}
	}
}

func TestReviewRetry_HappyPath(t *testing.T) {
	next, err := makeTask(TaskStatusInReview).RetryReview("fix the docs", []CriterionReview{
		{ID: "criterion-1", Satisfied: true},
		{ID: "criterion-2", Satisfied: false},
	}, fixedTime)
	if err != nil {
		t.Fatalf("RetryReview returned error: %v", err)
	}
	if next.Status != TaskStatusActive {
		t.Fatalf("status=%s want active", next.Status)
	}
	if next.Error != "fix the docs" {
		t.Fatalf("error=%q want feedback", next.Error)
	}
	if !next.AcceptanceCriteria[0].Satisfied {
		t.Fatal("first criterion should stay marked satisfied")
	}
	if next.AcceptanceCriteria[1].Satisfied {
		t.Fatal("second criterion should stay unsatisfied")
	}
}

func TestReviewRetry_RejectsEmptyFeedback(t *testing.T) {
	_, err := makeTask(TaskStatusInReview).RetryReview(" ", nil, fixedTime)
	if err == nil {
		t.Fatal("expected empty feedback retry to fail")
	}
}

func TestReviewFail_HappyPath(t *testing.T) {
	inReview := makeTask(TaskStatusInReview)
	inReview.ClaimedByMemberID = member.ID("member-fred")
	next, err := inReview.FailReview("criteria failed", []CriterionReview{
		{ID: "criterion-1", Satisfied: true},
		{ID: "criterion-2", Satisfied: false},
	}, fixedTime)
	if err != nil {
		t.Fatalf("FailReview returned error: %v", err)
	}
	if next.Status != TaskStatusFailed {
		t.Fatalf("status=%s want failed", next.Status)
	}
	if next.Error != "criteria failed" {
		t.Fatalf("error=%q want reason", next.Error)
	}
	if next.CompletedAt == nil {
		t.Fatal("CompletedAt should be stamped on failure")
	}
	if next.ClaimedBy() != member.ID("member-fred") {
		t.Fatalf("claim should be retained on failure for attribution, got %q", next.ClaimedBy())
	}
}

func TestReviewMethods_RejectNonReviewStatus(t *testing.T) {
	review := []CriterionReview{
		{ID: "criterion-1", Satisfied: true},
		{ID: "criterion-2", Satisfied: true},
	}
	if _, err := makeTask(TaskStatusActive).ApproveReview(review, fixedTime); err == nil {
		t.Fatal("expected approve from active to fail")
	}
	if _, err := makeTask(TaskStatusActive).RetryReview("again", review, fixedTime); err == nil {
		t.Fatal("expected retry from active to fail")
	}
	if _, err := makeTask(TaskStatusActive).FailReview("bad", review, fixedTime); err == nil {
		t.Fatal("expected fail from active to fail")
	}
}

func TestApproveReview_RequiresEveryCriterionSatisfied(t *testing.T) {
	_, err := makeTask(TaskStatusInReview).ApproveReview([]CriterionReview{
		{ID: "criterion-1", Satisfied: true},
		{ID: "criterion-2", Satisfied: false},
	}, fixedTime)
	if err == nil {
		t.Fatal("expected approval with unsatisfied criterion to fail")
	}
}

func TestReview_RejectsMissingOrUnknownCriteria(t *testing.T) {
	task := makeTask(TaskStatusInReview)
	if _, err := task.RetryReview("again", []CriterionReview{
		{ID: "criterion-1", Satisfied: true},
	}, fixedTime); err == nil {
		t.Fatal("expected partial criterion review to fail")
	}
	if _, err := task.RetryReview("again", []CriterionReview{
		{ID: "criterion-1", Satisfied: true},
		{ID: "criterion-x", Satisfied: true},
	}, fixedTime); err == nil {
		t.Fatal("expected unknown criterion review to fail")
	}
}

func TestBlockUnblock(t *testing.T) {
	blocked, err := makeTask(TaskStatusActive).Block("waiting on prerequisite task", fixedTime)
	if err != nil {
		t.Fatalf("Block returned error: %v", err)
	}
	if blocked.Status != TaskStatusBlocked {
		t.Fatalf("status=%s want blocked", blocked.Status)
	}
	active, err := blocked.Unblock("approved", fixedTime)
	if err != nil {
		t.Fatalf("Unblock returned error: %v", err)
	}
	if active.Status != TaskStatusActive {
		t.Fatalf("status=%s want active", active.Status)
	}
	if active.Error != "approved" {
		t.Fatalf("error=%q want approved", active.Error)
	}
}

func TestRelease(t *testing.T) {
	active := makeTask(TaskStatusActive)
	active.ClaimedByMemberID = member.ID("member-fred")
	active.ClaimedByMemberLabel = "Fred"
	next, err := active.Release(fixedTime)
	if err != nil {
		t.Fatalf("Release returned error: %v", err)
	}
	if next.Status != TaskStatusPending {
		t.Fatalf("status=%s want pending", next.Status)
	}
	if next.ClaimedBy() != "" {
		t.Fatalf("claim should be cleared")
	}
	if next.ClaimedByMemberLabel != "" {
		t.Fatalf("claim label should be cleared")
	}
}

func TestCancel(t *testing.T) {
	next, err := makeTask(TaskStatusActive).Cancel("obsolete", fixedTime)
	if err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	if next.Status != TaskStatusCanceled {
		t.Fatalf("status=%s want canceled", next.Status)
	}
	if next.CompletedAt == nil {
		t.Fatal("CompletedAt should be stamped")
	}
}

func TestTerminalTasksCannotTransition(t *testing.T) {
	terminal := makeTask(TaskStatusSucceeded)
	if _, err := terminal.Cancel("obsolete", fixedTime); err == nil {
		t.Fatal("expected cancel terminal to fail")
	}
}

func TestTransitions_DoNotMutateReceiver(t *testing.T) {
	pending := makeTask(TaskStatusPending)
	originalStatus := pending.Status
	originalClaim := pending.ClaimedBy()

	_, err := pending.Claim(member.ID("member-fred"), fixedTime)
	if err != nil {
		t.Fatalf("Claim returned error: %v", err)
	}
	if pending.Status != originalStatus {
		t.Fatalf("transition mutated status: got %s want %s", pending.Status, originalStatus)
	}
	if pending.ClaimedBy() != originalClaim {
		t.Fatalf("transition mutated claim: got %q want %q", pending.ClaimedBy(), originalClaim)
	}
}

func TestNewTask_HappyPath(t *testing.T) {
	task, err := NewTask(NewTaskInput{
		ProjectID:          types.ProjectID("project-1"),
		CreatedBy:          "coordinator",
		CreatedByLabel:     "Coordinator",
		AssignedTo:         member.ID("member-fred"),
		AssignedToLabel:    "Fred",
		Description:        "ship the thing",
		AcceptanceCriteria: []string{"done", "done", " "},
		Title:              "Ship",
	}, fixedTime)
	if err != nil {
		t.Fatalf("NewTask returned error: %v", err)
	}
	if task.Status != TaskStatusPending {
		t.Fatalf("status=%s want pending", task.Status)
	}
	if task.ProjectID != "project-1" {
		t.Fatalf("projectId=%s want project-1", task.ProjectID)
	}
	if task.AssignedMemberID() != "member-fred" {
		t.Fatalf("assignedTo=%q want member-fred", task.AssignedMemberID())
	}
	if task.AssignedToLabel != "Fred" {
		t.Fatalf("assignedToLabel=%q want Fred", task.AssignedToLabel)
	}
	if task.CreatedByLabel != "Coordinator" {
		t.Fatalf("createdByLabel=%q want Coordinator", task.CreatedByLabel)
	}
	if task.CleanDescription() != "ship the thing" {
		t.Fatalf("description=%q want ship the thing", task.CleanDescription())
	}
	if got := task.AcceptanceCriteriaCopy(); len(got) != 1 || got[0].ID != "criterion-1" || got[0].Text != "done" || got[0].Satisfied {
		t.Fatalf("acceptanceCriteria=%v want [done]", got)
	}
	if !strings.HasPrefix(string(task.ID), "task-") {
		t.Fatalf("taskID=%s should have task- prefix", task.ID)
	}
}

func TestNewTask_RejectsMissingRequiredFields(t *testing.T) {
	base := NewTaskInput{
		ProjectID:   types.ProjectID("project-1"),
		CreatedBy:   "coordinator",
		AssignedTo:  member.ID("member-fred"),
		Description: "do",
	}
	cases := []struct {
		name   string
		mutate func(NewTaskInput) NewTaskInput
		want   string
	}{
		{"empty ProjectID", func(in NewTaskInput) NewTaskInput { in.ProjectID = "  "; return in }, "project id"},
		{"empty CreatedBy", func(in NewTaskInput) NewTaskInput { in.CreatedBy = ""; return in }, "created by"},
		{"empty AssignedTo", func(in NewTaskInput) NewTaskInput { in.AssignedTo = ""; return in }, "assigned member id"},
		{"empty Description", func(in NewTaskInput) NewTaskInput { in.Description = ""; return in }, "description"},
	}
	for _, tc := range cases {
		_, err := NewTask(tc.mutate(base), fixedTime)
		if err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: error %q should mention %q", tc.name, err.Error(), tc.want)
		}
	}
}

func TestFixedClock_ReturnsConfiguredTime(t *testing.T) {
	c := FixedClock{T: fixedTime}
	if !c.Now().Equal(fixedTime) {
		t.Fatalf("Now()=%v want %v", c.Now(), fixedTime)
	}
}

func TestAttachArtifact_AppendsWithoutReplacing(t *testing.T) {
	task := makeTask(TaskStatusInReview)
	task.Artifacts = []string{"commit:abc123", "file:/project/docs/plan.md"}

	next, err := task.AttachArtifact("file:/project/.agen8/attachments/task-1/shot.png", fixedTime)
	if err != nil {
		t.Fatalf("AttachArtifact returned error: %v", err)
	}
	want := []string{"commit:abc123", "file:/project/docs/plan.md", "file:/project/.agen8/attachments/task-1/shot.png"}
	if len(next.Artifacts) != len(want) {
		t.Fatalf("artifacts=%v want %v", next.Artifacts, want)
	}
	for i := range want {
		if next.Artifacts[i] != want[i] {
			t.Fatalf("artifacts[%d]=%q want %q", i, next.Artifacts[i], want[i])
		}
	}
	if len(task.Artifacts) != 2 {
		t.Fatalf("original task mutated: %v", task.Artifacts)
	}
	if next.UpdatedAt == nil || !next.UpdatedAt.Equal(fixedTime) {
		t.Fatalf("updatedAt=%v want %v", next.UpdatedAt, fixedTime)
	}
}

func TestAttachArtifact_IsIdempotentForExistingRef(t *testing.T) {
	task := makeTask(TaskStatusActive)
	task.Artifacts = []string{"file:/project/.agen8/attachments/task-1/shot.png"}

	next, err := task.AttachArtifact("file:/project/.agen8/attachments/task-1/shot.png", fixedTime)
	if err != nil {
		t.Fatalf("AttachArtifact returned error: %v", err)
	}
	if len(next.Artifacts) != 1 {
		t.Fatalf("artifacts=%v want single entry", next.Artifacts)
	}
}

func TestAttachArtifact_RejectsCanceledTaskAndEmptyRef(t *testing.T) {
	if _, err := makeTask(TaskStatusCanceled).AttachArtifact("file:/project/x.png", fixedTime); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("expected canceled rejection, got %v", err)
	}
	if _, err := makeTask(TaskStatusActive).AttachArtifact("   ", fixedTime); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected empty-ref rejection, got %v", err)
	}
}
