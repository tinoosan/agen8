package task

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	fileapp "github.com/tinoosan/agen8/internal/services/file/app"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	taskapp "github.com/tinoosan/agen8/internal/services/task/app"
	taskdomain "github.com/tinoosan/agen8/internal/services/task/domain"
)

type Handler struct{}

func NewHandler() Handler {
	return Handler{}
}

func (h Handler) Handle(ctx context.Context, call CallContext, args json.RawMessage) (Result, error) {
	input, err := decode(args)
	if err != nil {
		return Result{}, err
	}
	if call.Tasks == nil {
		return Result{}, fmt.Errorf("task: task service is not configured")
	}
	if call.Members == nil {
		return Result{}, fmt.Errorf("task: member service is not configured")
	}
	ctx = contextWithSessionActor(ctx, call.ActorMemberID, call.ProjectID)
	actor, err := h.actor(ctx, call)
	if err != nil {
		return Result{}, err
	}
	taskCtx := caller.ContextWithCaller(ctx, caller.Caller{UserID: actor.UserID, MemberID: string(actor.MemberID), ProjectID: actor.ProjectID})

	switch input.Action {
	case "create":
		description, err := requireString(input.Description, "description")
		if err != nil {
			return Result{}, err
		}
		assignee, err := h.assignee(ctx, call, actor, input.AssigneeMemberID)
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Create(taskCtx, taskapp.CreateTaskParams{
			ProjectID:          actor.ProjectID,
			AssignedTo:         string(assignee.MemberID),
			Description:        description,
			AcceptanceCriteria: input.AcceptanceCriteria,
			Title:              input.Title,
			KeyResultRef:       input.KeyResultRef,
			MissionRef:         input.MissionRef,
			Metadata:           input.Metadata,
			TaskKind:           input.TaskKind,
		})
		return h.leanTaskResultForActor("create", task, err, nil, actor)
	case "get":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Get(taskCtx, id)
		if err == nil {
			err = h.canSeeTask(actor, task)
		}
		return h.fullTaskResult("get", task, err)
	case "list":
		filter, err := h.listFilter(actor, input)
		if err != nil {
			return Result{}, err
		}
		tasks, err := call.Tasks.List(taskCtx, filter)
		return h.listResult(tasks, err, input)
	case "claim":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Claim(taskCtx, id)
		return h.leanTaskResult("claim", task, err, nil)
	case "release":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Release(taskCtx, id)
		return h.leanTaskResult("release", task, err, nil)
	case "submit":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		summary, err := requireString(input.Summary, "summary")
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Complete(taskCtx, taskapp.CompleteTaskParams{TaskID: id, Summary: summary, Artifacts: input.Artifacts, Metadata: input.Metadata})
		return h.leanTaskResultForActor("submit", task, err, nil, actor)
	case "block":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		reason, err := requireString(input.Reason, "reason")
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Block(taskCtx, id, reason)
		return h.leanTaskResult("block", task, err, nil)
	case "unblock":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Unblock(taskCtx, id, input.Note)
		return h.leanTaskResult("unblock", task, err, nil)
	case "reassign":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		assignee, err := h.assignee(ctx, call, actor, input.AssigneeMemberID)
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Assign(taskCtx, taskapp.AssignTaskParams{TaskID: id, AssignedTo: string(assignee.MemberID)})
		return h.leanTaskResultForActor("reassign", task, err, nil, actor)
	case "update":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		params := taskapp.UpdateTaskParams{TaskID: id}
		if input.Title != "" {
			title := input.Title
			params.Title = &title
		}
		if input.Description != "" {
			description := input.Description
			params.Description = &description
		}
		if len(input.AcceptanceCriteria) > 0 {
			criteria := make([]taskdomain.AcceptanceCriterion, 0, len(input.AcceptanceCriteria))
			seen := map[string]struct{}{}
			for _, value := range input.AcceptanceCriteria {
				value = strings.TrimSpace(value)
				if value == "" {
					continue
				}
				if _, ok := seen[value]; ok {
					continue
				}
				seen[value] = struct{}{}
				criteria = append(criteria, taskdomain.AcceptanceCriterion{
					ID:   fmt.Sprintf("criterion-%d", len(criteria)+1),
					Text: value,
				})
			}
			if len(criteria) > 0 {
				params.AcceptanceCriteria = &criteria
			}
		}
		if input.TaskKind != "" {
			taskKind := input.TaskKind
			params.TaskKind = &taskKind
		}
		if input.KeyResultRef != "" {
			keyResultRef := input.KeyResultRef
			params.KeyResultRef = &keyResultRef
		}
		metadata := cloneRequestMetadata(input.Metadata)
		if missionRef := strings.TrimSpace(input.MissionRef); missionRef != "" {
			if metadata == nil {
				metadata = map[string]any{}
			}
			metadata["missionRef"] = missionRef
		}
		if len(metadata) > 0 {
			params.Metadata = metadata
		}
		task, err := call.Tasks.Update(taskCtx, params)
		return h.leanTaskResult("update", task, err, nil)
	case "cancel":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		reason, err := requireString(input.Reason, "reason")
		if err != nil {
			return Result{}, err
		}
		task, err := call.Tasks.Cancel(taskCtx, id, reason)
		return h.leanTaskResult("cancel", task, err, nil)
	case "review":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		params := taskapp.ReviewTaskParams{
			TaskID:   id,
			Reason:   input.Reason,
			Summary:  input.Summary,
			Note:     input.Note,
			Criteria: reviewCriteria(input.Criteria),
		}
		var task taskdomain.Task
		switch input.Decision {
		case "approve":
			task, err = call.Tasks.ApproveReview(taskCtx, params)
		case "retry":
			// Use a distinct name for the validation error so the outer review
			// `err` (read by taskResult below) actually receives the RetryReview
			// result. A `:=` here would shadow it and silently swallow the error.
			reason, verr := requireString(params.Reason, "reason")
			if verr != nil {
				return Result{}, verr
			}
			params.Reason = reason
			task, err = call.Tasks.RetryReview(taskCtx, params)
		case "fail":
			reason, verr := requireString(params.Reason, "reason")
			if verr != nil {
				return Result{}, verr
			}
			params.Reason = reason
			task, err = call.Tasks.FailReview(taskCtx, params)
		default:
			return Result{}, fmt.Errorf("task: decision must be approve, retry, or fail")
		}
		return h.leanTaskResult("review", task, err, map[string]any{"decision": input.Decision})
	case "attach":
		id, err := requireTaskID(input.TaskID)
		if err != nil {
			return Result{}, err
		}
		sources := 0
		for _, present := range []bool{input.Content != "", input.ContentB64 != "", input.FilePath != ""} {
			if present {
				sources++
			}
		}
		if sources != 1 {
			return Result{}, fmt.Errorf("task: attach requires exactly one of content, content_b64, or file_path")
		}
		// file_path lets the daemon read the bytes itself so they never round-trip
		// through the model. file_name then defaults to the path's base name.
		contentB64 := input.ContentB64
		if input.FilePath != "" {
			data, base, ferr := readAttachmentFile(input.FilePath, call.AttachmentRoots)
			if ferr != nil {
				return Result{}, ferr
			}
			contentB64 = base64.StdEncoding.EncodeToString(data)
			if input.FileName == "" {
				input.FileName = base
			}
		}
		fileName, err := requireAttachmentFileName(input.FileName)
		if err != nil {
			return Result{}, err
		}
		if call.Files == nil {
			return Result{}, fmt.Errorf("task: attach is not available: file store is not configured")
		}
		// Refuse before writing: uploading first and validating after would
		// orphan the file when the task cannot accept the artifact.
		loaded, err := call.Tasks.Get(taskCtx, id)
		if err != nil {
			return Result{}, err
		}
		if loaded.Status == taskdomain.TaskStatusCanceled {
			return Result{}, fmt.Errorf("task: cannot attach to canceled task %s", id)
		}
		vpath := attachmentVPath(id, fileName)
		uploaded, err := call.Files.Upload(taskCtx, fileapp.UploadInput{
			ProjectID: types.ProjectID(call.ProjectID),
			Path:      vpath,
			Content:   input.Content,
			BytesB64:  contentB64,
		})
		if err != nil {
			return Result{}, fmt.Errorf("task: attach upload: %w", err)
		}
		ref := "file:" + uploaded.Path
		task, err := call.Tasks.AttachArtifact(taskCtx, id, ref)
		if err != nil {
			// Best-effort cleanup so a failed append does not orphan the upload.
			_, _ = call.Files.Delete(taskCtx, fileapp.PathInput{ProjectID: types.ProjectID(call.ProjectID), Path: uploaded.Path})
			return Result{}, err
		}
		return h.leanTaskResult("attach", task, nil, map[string]any{"artifactRef": ref})
	default:
		return Result{}, fmt.Errorf("task: unsupported action %q", input.Action)
	}
}

// requireAttachmentFileName accepts only a bare file name. Anything that
// could traverse out of the task's attachment directory (separators, "..")
// is rejected rather than normalized so the caller's mistake stays visible.
func requireAttachmentFileName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", fmt.Errorf("task: file_name is required")
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return "", fmt.Errorf("task: file_name must be a bare file name without path separators or \"..\"")
	}
	if name == "." {
		return "", fmt.Errorf("task: file_name must be a bare file name without path separators or \"..\"")
	}
	return name, nil
}

func attachmentVPath(id taskdomain.TaskID, fileName string) string {
	return "/project/.agen8/attachments/" + string(id) + "/" + fileName
}

// maxAttachmentFileBytes caps file_path attachments. Inline content is already
// bounded by the RPC body limit; this bound covers daemon-side reads.
const maxAttachmentFileBytes = 25 << 20 // 25 MiB

// readAttachmentFile reads an attachment source from the daemon host's
// filesystem so the bytes never round-trip through the model. The source is
// copied, never moved — the caller's file is left untouched.
func readAttachmentFile(path string, allowedRoots []string) (data []byte, baseName string, err error) {
	if !filepath.IsAbs(path) {
		return nil, "", fmt.Errorf("task: file_path must be an absolute path, got %q", path)
	}
	rootPath, relativePath, err := attachmentSourceRoot(path, allowedRoots)
	if err != nil {
		return nil, "", err
	}
	name := filepath.Base(relativePath)
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, "", fmt.Errorf("task: file_path: %w", err)
	}
	defer root.Close()
	linkInfo, err := root.Lstat(relativePath)
	if err != nil {
		return nil, "", fmt.Errorf("task: file_path: %w", err)
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		return nil, "", fmt.Errorf("task: file_path must not be a symlink")
	}
	file, err := root.Open(relativePath)
	if err != nil {
		return nil, "", fmt.Errorf("task: file_path: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("task: file_path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("task: file_path must be a regular file, %q is not", path)
	}
	if info.Size() > maxAttachmentFileBytes {
		return nil, "", fmt.Errorf("task: file_path: %q is %d bytes, over the %d byte attachment limit", path, info.Size(), maxAttachmentFileBytes)
	}
	data, err = io.ReadAll(file)
	if err != nil {
		return nil, "", fmt.Errorf("task: file_path: %w", err)
	}
	return data, name, nil
}

func attachmentSourceRoot(path string, allowedRoots []string) (root string, relative string, err error) {
	path = filepath.Clean(path)
	for _, candidate := range allowedRoots {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		candidate, err = filepath.Abs(candidate)
		if err != nil {
			continue
		}
		relative, err = filepath.Rel(filepath.Clean(candidate), path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		return filepath.Clean(candidate), relative, nil
	}
	return "", "", fmt.Errorf("task: file_path must be within an approved project root")
}

type actor struct {
	MemberID   member.ID       `json:"memberId"`
	UserID     string          `json:"userId"`
	ProjectID  types.ProjectID `json:"projectId"`
	MemberType string          `json:"memberType"`
	Label      string          `json:"label"`
}

func contextWithSessionActor(ctx context.Context, actorMemberID, projectID string) context.Context {
	actorMemberID = strings.TrimSpace(actorMemberID)
	projectID = strings.TrimSpace(projectID)
	if actorMemberID == "" && projectID == "" {
		return ctx
	}
	return caller.ContextWithCaller(ctx, caller.Caller{
		MemberID:  actorMemberID,
		ProjectID: types.ProjectID(projectID),
	})
}

func (h Handler) actor(ctx context.Context, call CallContext) (actor, error) {
	memberID := strings.TrimSpace(call.ActorMemberID)
	if memberID == "" {
		return actor{}, fmt.Errorf("task: registered member_id is required")
	}
	rosterMember, err := call.Members.GetMember(ctx, member.ID(memberID))
	if err != nil {
		return actor{}, fmt.Errorf("task: load registered member: %w", err)
	}
	if strings.TrimSpace(rosterMember.LifecycleState) != "" && !strings.EqualFold(rosterMember.LifecycleState, member.LifecycleActive) {
		return actor{}, fmt.Errorf("task: registered member %q is not active", memberID)
	}
	projectID := strings.TrimSpace(call.ProjectID)
	rosterProjectID := strings.TrimSpace(rosterMember.ProjectID)
	if projectID == "" {
		projectID = rosterProjectID
	} else if rosterProjectID != "" && projectID != rosterProjectID {
		return actor{}, fmt.Errorf("task: registered member %q is not in project %q", memberID, projectID)
	}
	if projectID == "" {
		return actor{}, fmt.Errorf("task: registered member project is required")
	}
	return actor{
		MemberID:   member.ID(memberID),
		UserID:     strings.TrimSpace(rosterMember.UserID),
		ProjectID:  types.ProjectID(projectID),
		MemberType: strings.TrimSpace(rosterMember.MemberType),
		Label:      memberLabel(rosterMember),
	}, nil
}

func (h Handler) assignee(ctx context.Context, call CallContext, caller actor, memberID string) (actor, error) {
	memberID = strings.TrimSpace(memberID)
	if memberID == "" {
		return actor{}, fmt.Errorf("task: assignee_member_id is required")
	}
	rosterMember, err := call.Members.GetMember(ctx, member.ID(memberID))
	if err != nil {
		return actor{}, fmt.Errorf("task: load assignee member: %w", err)
	}
	if strings.TrimSpace(rosterMember.LifecycleState) != "" && !strings.EqualFold(rosterMember.LifecycleState, member.LifecycleActive) {
		return actor{}, fmt.Errorf("task: assignee member %q is not active", memberID)
	}
	if strings.TrimSpace(rosterMember.ProjectID) != strings.TrimSpace(string(caller.ProjectID)) {
		return actor{}, fmt.Errorf("task: assignee member %q belongs to project %q, not %q", memberID, rosterMember.ProjectID, caller.ProjectID)
	}
	return actor{
		MemberID:   member.ID(memberID),
		ProjectID:  types.ProjectID(strings.TrimSpace(rosterMember.ProjectID)),
		MemberType: strings.TrimSpace(rosterMember.MemberType),
		Label:      memberLabel(rosterMember),
	}, nil
}

func (h Handler) listFilter(caller actor, input requestInput) (taskdomain.TaskFilter, error) {
	filter := taskdomain.TaskFilter{
		ProjectID: types.ProjectID(strings.TrimSpace(string(caller.ProjectID))),
		Limit:     input.Limit,
		Offset:    input.Offset,
	}
	if input.Status != "" {
		status, err := parseStatus(input.Status)
		if err != nil {
			return taskdomain.TaskFilter{}, err
		}
		filter.Status = []taskdomain.TaskStatus{status}
	}
	if input.AssigneeMemberID != "" {
		filter.AssignedTo = member.ID(input.AssigneeMemberID)
	}
	return filter, nil
}

func (h Handler) canSeeTask(caller actor, task taskdomain.Task) error {
	if task.ProjectID != caller.ProjectID {
		return fmt.Errorf("task: task %s is not visible in project %s", task.ID, caller.ProjectID)
	}
	return nil
}

func requireTaskID(value string) (taskdomain.TaskID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("task: task_id is required")
	}
	return taskdomain.TaskID(value), nil
}

func requireString(value, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("task: %s is required", field)
	}
	return value, nil
}

func parseStatus(value string) (taskdomain.TaskStatus, error) {
	switch taskdomain.TaskStatus(strings.TrimSpace(value)) {
	case taskdomain.TaskStatusPending, taskdomain.TaskStatusActive, taskdomain.TaskStatusBlocked, taskdomain.TaskStatusInReview, taskdomain.TaskStatusSucceeded, taskdomain.TaskStatusFailed, taskdomain.TaskStatusCanceled:
		return taskdomain.TaskStatus(value), nil
	default:
		return "", fmt.Errorf("task: unsupported status %q", value)
	}
}

func cloneRequestMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func reviewCriteria(values []reviewCriterionInput) []taskdomain.CriterionReview {
	if len(values) == 0 {
		return nil
	}
	out := make([]taskdomain.CriterionReview, 0, len(values))
	for _, value := range values {
		out = append(out, taskdomain.CriterionReview{ID: strings.TrimSpace(value.ID), Satisfied: value.Satisfied})
	}
	return out
}
