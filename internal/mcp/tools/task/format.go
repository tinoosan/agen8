package task

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	taskdomain "github.com/tinoosan/agen8/internal/services/task/domain"
)

const missionDirectTaskCreateAdvisory = "This task is not linked to a key result. It can still be created, but mission progress will not reflect it until it is linked to a key result."

// taskEntry is the FULL detail returned only by the `get` action — the explicit
// "I want the details" fetch. Mutations and list return the leaner shapes below.
// Card-era display labels (assignedToLabel/claimedByMemberLabel/createdByLabel)
// and the raw metadata echo were dropped: nothing renders them now (the model
// routes by id), missionRef is surfaced as its own field, and block/fail reasons
// ride statusReason.
type taskEntry struct {
	ID                 string                           `json:"id"`
	Status             string                           `json:"status,omitempty"`
	StatusReason       string                           `json:"statusReason,omitempty"`
	Title              string                           `json:"title,omitempty"`
	Description        string                           `json:"description,omitempty"`
	AssignedToMemberID string                           `json:"assignedToMemberId,omitempty"`
	ClaimedByMemberID  string                           `json:"claimedByMemberId,omitempty"`
	CreatedBy          string                           `json:"createdBy,omitempty"`
	KeyResultRef       string                           `json:"keyResultRef,omitempty"`
	MissionRef         string                           `json:"missionRef,omitempty"`
	TaskKind           string                           `json:"taskKind,omitempty"`
	AcceptanceCriteria []taskdomain.AcceptanceCriterion `json:"acceptanceCriteria,omitempty"`
	Summary            string                           `json:"summary,omitempty"`
	Artifacts          []string                         `json:"artifacts,omitempty"`
}

// taskAck is the mutation response — just what changed. The caller already holds
// the title, refs, kind, and member ids (it created the task or is acting on one
// it's working), so a mutation only needs to confirm the new status (and any
// status reason). Detail is a `get` away.
type taskAck struct {
	ID           string `json:"id"`
	Status       string `json:"status,omitempty"`
	StatusReason string `json:"statusReason,omitempty"`
}

// leanTaskEntry is a list/scan row: enough to recognize and triage a task
// without fetching each one. Mutations use taskAck; get returns the full entry.
type leanTaskEntry struct {
	ID                 string `json:"id"`
	Status             string `json:"status,omitempty"`
	Title              string `json:"title,omitempty"`
	AssignedToMemberID string `json:"assignedToMemberId,omitempty"`
}

func encodeTaskResponse(action string, entry any, extra map[string]any) (Result, error) {
	structured := map[string]any{
		"ok":     true,
		"tool":   Name,
		"action": action,
		"task":   entry,
	}
	for key, value := range extra {
		structured[key] = value
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

// fullTaskResult returns the detailed entry — used only by the `get` action.
func (h Handler) fullTaskResult(action string, task taskdomain.Task, err error) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	return encodeTaskResponse(action, toTaskEntry(task), nil)
}

// leanTaskResult returns the mutation ack — used by every mutation action so the
// model isn't re-sent the task it just acted on.
func (h Handler) leanTaskResult(action string, task taskdomain.Task, err error, extra map[string]any) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	return encodeTaskResponse(action, toTaskAck(task), extra)
}

// leanTaskResultForActor adds the actor-specific nextAction/guidance hints, then
// returns the mutation ack (create / submit / reassign).
func (h Handler) leanTaskResultForActor(action string, task taskdomain.Task, err error, extra map[string]any, actor actor) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	nextAction, guidance := taskResponseGuidance(action, task, actor.MemberID)
	if nextAction != "" || guidance != "" {
		if extra == nil {
			extra = map[string]any{}
		}
		if nextAction != "" {
			extra["nextAction"] = nextAction
		}
		if guidance != "" {
			extra["guidance"] = guidance
		}
	}
	return h.leanTaskResult(action, task, nil, extra)
}

func taskResponseGuidance(action string, task taskdomain.Task, actorID member.ID) (string, string) {
	actorID = member.ID(strings.TrimSpace(string(actorID)))
	if actorID == "" {
		return "", ""
	}
	switch action {
	case "create":
		nextAction, guidance := "", ""
		if task.Status == taskdomain.TaskStatusPending && task.AssignedTo == actorID {
			nextAction = "claim"
			guidance = "Claim the task before starting work. Fetch the task when you need the full description and acceptance criteria."
		}
		if strings.TrimSpace(task.KeyResultRef) == "" {
			guidance = appendGuidance(guidance, missionDirectTaskCreateAdvisory)
		}
		return nextAction, guidance
	case "reassign":
		if task.Status == taskdomain.TaskStatusPending && task.AssignedTo == actorID {
			return "claim", "Claim the task before starting work. Fetch the task when you need the full description and acceptance criteria."
		}
	case "submit":
		if task.Status == taskdomain.TaskStatusInReview && member.ID(strings.TrimSpace(task.CreatedBy)) == actorID {
			return "review", "Fetch the task, inspect the submitted work against the acceptance criteria, then approve, retry, or fail the review. Include summary and note fields as needed — concise summary, detailed note, or both are persisted in task metadata even when approved."
		}
	}
	return "", ""
}

func appendGuidance(existing, advisory string) string {
	existing = strings.TrimSpace(existing)
	advisory = strings.TrimSpace(advisory)
	if existing == "" {
		return advisory
	}
	if advisory == "" {
		return existing
	}
	return existing + " " + advisory
}

func (h Handler) listResult(tasks []taskdomain.Task, err error, input requestInput) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	rows := make([]leanTaskEntry, 0, len(tasks))
	for _, task := range tasks {
		rows = append(rows, toLeanTaskEntry(task))
	}
	structured := map[string]any{
		"ok":     true,
		"tool":   Name,
		"action": "list",
		"tasks":  rows,
		"count":  len(rows),
		"limit":  input.Limit,
		"offset": input.Offset,
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func toTaskAck(task taskdomain.Task) taskAck {
	return taskAck{
		ID:           strings.TrimSpace(string(task.ID)),
		Status:       strings.TrimSpace(string(task.Status)),
		StatusReason: strings.TrimSpace(task.Error),
	}
}

func toLeanTaskEntry(task taskdomain.Task) leanTaskEntry {
	return leanTaskEntry{
		ID:                 strings.TrimSpace(string(task.ID)),
		Status:             strings.TrimSpace(string(task.Status)),
		Title:              strings.TrimSpace(task.Title),
		AssignedToMemberID: strings.TrimSpace(string(task.AssignedTo)),
	}
}

func toTaskEntry(task taskdomain.Task) taskEntry {
	return taskEntry{
		ID:                 strings.TrimSpace(string(task.ID)),
		Status:             strings.TrimSpace(string(task.Status)),
		StatusReason:       strings.TrimSpace(task.Error),
		Title:              strings.TrimSpace(task.Title),
		Description:        strings.TrimSpace(task.Description),
		AssignedToMemberID: strings.TrimSpace(string(task.AssignedTo)),
		ClaimedByMemberID:  strings.TrimSpace(string(task.ClaimedByMemberID)),
		CreatedBy:          strings.TrimSpace(task.CreatedBy),
		KeyResultRef:       strings.TrimSpace(task.KeyResultRef),
		MissionRef:         missionRefFromTaskMetadata(task.Metadata),
		TaskKind:           strings.TrimSpace(task.TaskKind),
		AcceptanceCriteria: append([]taskdomain.AcceptanceCriterion(nil), task.AcceptanceCriteria...),
		Summary:            strings.TrimSpace(task.Summary),
		Artifacts:          append([]string(nil), task.Artifacts...),
	}
}

func missionRefFromTaskMetadata(metadata map[string]any) string {
	for _, key := range []string{"missionRef", "mission_ref", "missionId", "mission_id"} {
		raw, ok := metadata[key]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			return ""
		}
		return strings.TrimSpace(value)
	}
	return ""
}

func memberLabel(member member.Record) string {
	if strings.TrimSpace(member.DisplayName) != "" {
		return strings.TrimSpace(member.DisplayName)
	}
	if strings.TrimSpace(member.HarnessKind) != "" {
		return strings.TrimSpace(member.HarnessKind)
	}
	if strings.TrimSpace(member.MemberType) != "" {
		return strings.ReplaceAll(strings.TrimSpace(member.MemberType), "_", " ")
	}
	return ""
}

func encodeText(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("task: encode structured response: %w", err)
	}
	return string(encoded), nil
}
