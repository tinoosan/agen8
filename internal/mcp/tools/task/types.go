package task

import (
	"context"
	"encoding/json"

	fileapp "github.com/tinoosan/agen8/internal/services/file/app"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	taskapp "github.com/tinoosan/agen8/internal/services/task/app"
	taskdomain "github.com/tinoosan/agen8/internal/services/task/domain"
)

type Service interface {
	Create(context.Context, taskapp.CreateTaskParams) (taskdomain.Task, error)
	Get(context.Context, taskdomain.TaskID) (taskdomain.Task, error)
	List(context.Context, taskdomain.TaskFilter) ([]taskdomain.Task, error)
	Claim(context.Context, taskdomain.TaskID) (taskdomain.Task, error)
	Release(context.Context, taskdomain.TaskID) (taskdomain.Task, error)
	Complete(context.Context, taskapp.CompleteTaskParams) (taskdomain.Task, error)
	Update(context.Context, taskapp.UpdateTaskParams) (taskdomain.Task, error)
	Block(context.Context, taskdomain.TaskID, string) (taskdomain.Task, error)
	Unblock(context.Context, taskdomain.TaskID, string) (taskdomain.Task, error)
	Assign(context.Context, taskapp.AssignTaskParams) (taskdomain.Task, error)
	Cancel(context.Context, taskdomain.TaskID, string) (taskdomain.Task, error)
	ApproveReview(context.Context, taskapp.ReviewTaskParams) (taskdomain.Task, error)
	RetryReview(context.Context, taskapp.ReviewTaskParams) (taskdomain.Task, error)
	FailReview(context.Context, taskapp.ReviewTaskParams) (taskdomain.Task, error)
	AttachArtifact(ctx context.Context, taskID taskdomain.TaskID, ref string) (taskdomain.Task, error)
}

type MemberDirectory interface {
	GetMember(ctx context.Context, id member.ID) (member.Record, error)
}

// FileStore is the slice of the file service the attach action needs: write
// the uploaded bytes, and remove them again if the artifact append fails.
type FileStore interface {
	Upload(ctx context.Context, input fileapp.UploadInput) (fileapp.PathResult, error)
	Delete(ctx context.Context, input fileapp.PathInput) (struct{}, error)
}

type CallContext struct {
	Tasks         Service
	Members       MemberDirectory
	Files         FileStore
	ProjectID     string
	ActorMemberID string
}

type Result struct {
	Text       string
	Structured any
}

type reviewCriterionInput struct {
	ID        string `json:"id"`
	Satisfied bool   `json:"satisfied"`
}

type rawRequest struct {
	Action             string                 `json:"action"`
	TaskID             *string                `json:"task_id"`
	AssigneeMemberID   *string                `json:"assignee_member_id"`
	Status             *string                `json:"status"`
	Title              *string                `json:"title"`
	Limit              *int                   `json:"limit"`
	Offset             *int                   `json:"offset"`
	Metadata           json.RawMessage        `json:"metadata"`
	AcceptanceCriteria []string               `json:"acceptance_criteria"`
	KeyResultRef       *string                `json:"key_result_ref"`
	MissionRef         *string                `json:"mission_ref"`
	Description        *string                `json:"description"`
	TaskKind           *string                `json:"task_kind"`
	Summary            *string                `json:"summary"`
	Artifacts          []string               `json:"artifacts"`
	Reason             *string                `json:"reason"`
	Note               *string                `json:"note"`
	Decision           *string                `json:"decision"`
	Criteria           []reviewCriterionInput `json:"criteria"`
	FileName           *string                `json:"file_name"`
	Content            *string                `json:"content"`
	ContentB64         *string                `json:"content_b64"`
	FilePath           *string                `json:"file_path"`
}

type requestInput struct {
	Action             string
	TaskID             string
	AssigneeMemberID   string
	Status             string
	Title              string
	Limit              int
	Offset             int
	Metadata           map[string]any
	AcceptanceCriteria []string
	KeyResultRef       string
	MissionRef         string
	Description        string
	TaskKind           string
	Summary            string
	Artifacts          []string
	Reason             string
	Note               string
	Decision           string
	Criteria           []reviewCriterionInput
	FileName           string
	Content            string
	ContentB64         string
	FilePath           string
}
