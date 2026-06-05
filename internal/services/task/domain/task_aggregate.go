package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

type CriterionReview struct {
	ID        string
	Satisfied bool
}

func (t Task) IsTerminal() bool            { return t.Status.IsTerminal() }
func (t Task) AssignedMemberID() member.ID { return trimMemberID(t.AssignedTo) }
func (t Task) ClaimedBy() member.ID        { return trimMemberID(t.ClaimedByMemberID) }
func (t Task) CleanDescription() string    { return strings.TrimSpace(t.Description) }
func (t Task) AcceptanceCriteriaCopy() []AcceptanceCriterion {
	return append([]AcceptanceCriterion(nil), t.AcceptanceCriteria...)
}

// Claim transitions a pending task to active for its assigned member.
func (t Task) Claim(memberID member.ID, now time.Time) (Task, error) {
	memberID = trimMemberID(memberID)
	if memberID == "" {
		return Task{}, fmt.Errorf("claim: member id is required")
	}
	if t.Status != TaskStatusPending {
		return Task{}, fmt.Errorf("claim: task %s cannot be claimed from status %s: must be %s",
			t.ID, t.Status, TaskStatusPending)
	}
	if assignedTo := t.AssignedMemberID(); assignedTo != "" && assignedTo != memberID {
		return Task{}, fmt.Errorf("claim: task %s is assigned to %s, not %s",
			t.ID, assignedTo, memberID)
	}
	next := t
	next.Status = TaskStatusActive
	next.ClaimedByMemberID = memberID
	stampUpdated(&next, now)
	if next.StartedAt == nil {
		started := now.UTC()
		next.StartedAt = &started
	}
	return next, nil
}

// Assign routes any non-terminal task to a member and makes it claimable.
func (t Task) Assign(memberID member.ID, now time.Time) (Task, error) {
	memberID = trimMemberID(memberID)
	if memberID == "" {
		return Task{}, fmt.Errorf("assign: member id is required")
	}
	if t.Status.IsTerminal() {
		return Task{}, fmt.Errorf("assign: task %s is terminal (%s)", t.ID, t.Status)
	}
	next := t
	next.AssignedTo = memberID
	next.ClaimedByMemberID = ""
	next.Status = TaskStatusPending
	next.Error = ""
	stampUpdated(&next, now)
	return next, nil
}

// Complete moves active worker output into coordinator review.
func (t Task) Complete(summary string, artifacts []string, now time.Time) (Task, error) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return Task{}, fmt.Errorf("complete: summary is required")
	}
	if t.Status != TaskStatusActive {
		return Task{}, fmt.Errorf("complete: task %s cannot be completed from status %s: must be %s",
			t.ID, t.Status, TaskStatusActive)
	}
	if t.ClaimedBy() == "" {
		return Task{}, fmt.Errorf("complete: task %s has no claiming member", t.ID)
	}

	next := t
	next.Status = TaskStatusInReview
	next.Summary = summary
	next.Artifacts = cleanArtifactList(artifacts)
	next.Error = ""
	stampUpdated(&next, now)
	return next, nil
}

// ApproveReview accepts an in-review task after every criterion is satisfied.
func (t Task) ApproveReview(criteria []CriterionReview, now time.Time) (Task, error) {
	if t.Status != TaskStatusInReview {
		return Task{}, fmt.Errorf("approve review: task %s cannot be approved from status %s: must be %s",
			t.ID, t.Status, TaskStatusInReview)
	}
	nextCriteria, err := t.reviewCriteria(criteria)
	if err != nil {
		return Task{}, fmt.Errorf("approve review: %w", err)
	}
	for _, criterion := range nextCriteria {
		if !criterion.Satisfied {
			return Task{}, fmt.Errorf("approve review: criterion %s is not satisfied", criterion.ID)
		}
	}
	next := t
	next.Status = TaskStatusSucceeded
	next.AcceptanceCriteria = nextCriteria
	next.Error = ""
	next.ClaimedByMemberID = ""
	completed := now.UTC()
	next.CompletedAt = &completed
	next.UpdatedAt = &completed
	return next, nil
}

// RetryReview sends an in-review task back to the assigned worker.
func (t Task) RetryReview(feedback string, criteria []CriterionReview, now time.Time) (Task, error) {
	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		return Task{}, fmt.Errorf("retry review: feedback is required")
	}
	if t.Status != TaskStatusInReview {
		return Task{}, fmt.Errorf("retry review: task %s cannot be retried from status %s: must be %s",
			t.ID, t.Status, TaskStatusInReview)
	}
	nextCriteria, err := t.reviewCriteria(criteria)
	if err != nil {
		return Task{}, fmt.Errorf("retry review: %w", err)
	}
	next := t
	next.Status = TaskStatusActive
	next.AcceptanceCriteria = nextCriteria
	next.Error = feedback
	stampUpdated(&next, now)
	return next, nil
}

// FailReview rejects an in-review task as failed.
func (t Task) FailReview(reason string, criteria []CriterionReview, now time.Time) (Task, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Task{}, fmt.Errorf("fail review: reason is required")
	}
	if t.Status != TaskStatusInReview {
		return Task{}, fmt.Errorf("fail review: task %s cannot be failed from status %s: must be %s",
			t.ID, t.Status, TaskStatusInReview)
	}
	nextCriteria, err := t.reviewCriteria(criteria)
	if err != nil {
		return Task{}, fmt.Errorf("fail review: %w", err)
	}
	next := t
	next.Status = TaskStatusFailed
	next.AcceptanceCriteria = nextCriteria
	next.Error = reason
	next.ClaimedByMemberID = ""
	completed := now.UTC()
	next.CompletedAt = &completed
	next.UpdatedAt = &completed
	return next, nil
}

// Block records that an active task is waiting on something external.
func (t Task) Block(reason string, now time.Time) (Task, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Task{}, fmt.Errorf("block: reason is required")
	}
	if t.Status != TaskStatusActive {
		return Task{}, fmt.Errorf("block: task %s cannot be blocked from status %s: must be %s",
			t.ID, t.Status, TaskStatusActive)
	}
	next := t
	next.Status = TaskStatusBlocked
	next.Error = reason
	stampUpdated(&next, now)
	return next, nil
}

// Unblock returns a blocked task to active work.
func (t Task) Unblock(note string, now time.Time) (Task, error) {
	if t.Status != TaskStatusBlocked {
		return Task{}, fmt.Errorf("unblock: task %s cannot be unblocked from status %s: must be %s",
			t.ID, t.Status, TaskStatusBlocked)
	}
	next := t
	next.Status = TaskStatusActive
	next.Error = strings.TrimSpace(note)
	stampUpdated(&next, now)
	return next, nil
}

// Release drops the current claim and returns the task to pending.
func (t Task) Release(now time.Time) (Task, error) {
	if t.Status != TaskStatusActive {
		return Task{}, fmt.Errorf("release: task %s cannot be released from status %s: must be %s",
			t.ID, t.Status, TaskStatusActive)
	}
	next := t
	next.Status = TaskStatusPending
	next.ClaimedByMemberID = ""
	stampUpdated(&next, now)
	return next, nil
}

// Cancel ends any non-terminal task.
func (t Task) Cancel(reason string, now time.Time) (Task, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Task{}, fmt.Errorf("cancel: reason is required")
	}
	if t.Status.IsTerminal() {
		return Task{}, fmt.Errorf("cancel: task %s is terminal (%s)", t.ID, t.Status)
	}
	next := t
	next.Status = TaskStatusCanceled
	next.Error = reason
	next.ClaimedByMemberID = ""
	completed := now.UTC()
	next.CompletedAt = &completed
	next.UpdatedAt = &completed
	return next, nil
}

func (t Task) reviewCriteria(reviews []CriterionReview) ([]AcceptanceCriterion, error) {
	if len(t.AcceptanceCriteria) == 0 {
		if len(reviews) != 0 {
			return nil, fmt.Errorf("task has no acceptance criteria")
		}
		return nil, nil
	}
	if len(reviews) != len(t.AcceptanceCriteria) {
		return nil, fmt.Errorf("review must include every acceptance criterion")
	}

	byID := make(map[string]bool, len(reviews))
	for _, review := range reviews {
		id := strings.TrimSpace(review.ID)
		if id == "" {
			return nil, fmt.Errorf("criterion id is required")
		}
		if _, exists := byID[id]; exists {
			return nil, fmt.Errorf("criterion %s reviewed more than once", id)
		}
		byID[id] = review.Satisfied
	}

	next := make([]AcceptanceCriterion, len(t.AcceptanceCriteria))
	for i, criterion := range t.AcceptanceCriteria {
		satisfied, ok := byID[criterion.ID]
		if !ok {
			return nil, fmt.Errorf("criterion %s was not reviewed", criterion.ID)
		}
		next[i] = criterion
		next[i].Satisfied = satisfied
	}
	return next, nil
}

func stampUpdated(task *Task, now time.Time) {
	updated := now.UTC()
	task.UpdatedAt = &updated
}

func trimMemberID(id member.ID) member.ID {
	return member.ID(strings.TrimSpace(string(id)))
}

// cleanArtifactList trims, drops empties, and dedupes. Order is preserved.
func cleanArtifactList(artifacts []string) []string {
	return cleanStringList(artifacts)
}
