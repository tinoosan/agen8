package project

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

const Name = "project"
const Description = "[COORDINATION] Project gateway for workspace registration and member roster CRUD. Call action=register first when the MCP session is not yet bound to a project/member."

var allActions = []string{"register", "member_create", "member_get", "member_list", "member_update_config", "member_remove"}

func BootstrapActionAllowed(action string) bool {
	return strings.TrimSpace(strings.ToLower(action)) == "register"
}

type MemberService interface {
	GetMember(ctx context.Context, id member.ID) (member.Record, error)
	ListMembers(ctx context.Context, filter member.Filter) ([]member.Record, error)
}

type MemberRegistrar interface {
	RegisterMember(ctx context.Context, rosterMember member.Record) (projectapp.RegisterMemberResult, error)
	UpdateMemberConfig(ctx context.Context, id member.ID, model, effort, harnessKind string, permissionFields ...string) (member.Record, error)
	RemoveMember(ctx context.Context, id member.ID) (member.Record, error)
}

type ContextRegistrar interface {
	RegisterMCPContext(ctx context.Context, req RegisterContextRequest) (RegisterContextResult, error)
}

type RegisterContextRequest struct {
	Token string
	// BoundProjectID is the session's server-side project binding (from a wlt_
	// link token). Authoritative: it overrides the caller-asserted ProjectID.
	BoundProjectID   string
	ProjectID        string
	ProjectRoot      string
	LocationID       string
	DisplayName      string
	HarnessKind      string
	SessionID        string
	ThreadID         string
	NativeSessionRef string
	Model            string
	Effort           string
	PermissionMode   string
	ConfigRef        string
}

type RegisterContextResult struct {
	ProjectID        string
	ProjectRoot      string
	LocationID       string
	MemberID         string
	DisplayName      string
	MemberType       string
	ChannelID        string
	SessionID        string
	ThreadID         string
	NativeSessionRef string
	Token            string
	URL              string
	MCPServers       []string
}

type CallContext struct {
	Members          MemberService
	Registrar        MemberRegistrar
	ContextRegistrar ContextRegistrar
	MCPToken         string
	UserID           string
	HarnessKind      string
	ProjectID        string
	ActorMemberID    string
	SessionID        string
	ThreadID         string
}

type Result struct {
	Text       string
	Structured any
}

type Handler struct{}

func NewHandler() Handler {
	return Handler{}
}

func (h Handler) Schema() json.RawMessage {
	return mustSchema(allActions)
}

func (h Handler) Handle(ctx context.Context, call CallContext, args json.RawMessage) (Result, error) {
	input, err := decode(args)
	if err != nil {
		return Result{}, err
	}
	if input.Action == "register" {
		// Register dispatches before the gap-fill below so the caller-asserted
		// input.ProjectID stays distinct from the session binding call.ProjectID:
		// the binding is authoritative and must not be silently merged away.
		return h.registerContext(ctx, call, input)
	}
	if input.ProjectID == "" {
		input.ProjectID = strings.TrimSpace(call.ProjectID)
	}
	actor, ctx, err := h.resolveActor(ctx, call)
	if err != nil {
		return Result{}, err
	}
	projectID := strings.TrimSpace(actor.ProjectID)
	if input.Action == "member_create" || input.Action == "member_update_config" || input.Action == "member_remove" {
		if err := requireCoordinatorActor(actor, input.Action); err != nil {
			return Result{}, err
		}
	}
	switch input.Action {
	case "member_list":
		return h.memberList(ctx, call, projectID)
	case "member_get":
		return h.memberGet(ctx, call, input.MemberID)
	case "member_create":
		return h.createMember(ctx, call, projectID, input)
	case "member_update_config":
		return h.memberUpdateConfig(ctx, call, input)
	case "member_remove":
		return h.memberRemove(ctx, call, input.MemberID)
	default:
		return Result{}, fmt.Errorf("project: unsupported action %q", input.Action)
	}
}

func (h Handler) resolveActor(ctx context.Context, call CallContext) (member.Record, context.Context, error) {
	if call.Members == nil {
		return member.Record{}, ctx, fmt.Errorf("project: member service is not configured")
	}
	memberID := strings.TrimSpace(call.ActorMemberID)
	if memberID == "" {
		return member.Record{}, ctx, fmt.Errorf("project: actor member id is required")
	}
	ctx = caller.ContextWithCaller(ctx, caller.Caller{
		MemberID:  member.ID(memberID),
		ProjectID: types.ProjectID(strings.TrimSpace(call.ProjectID)),
	})
	actor, err := call.Members.GetMember(ctx, member.ID(memberID))
	if err != nil {
		return member.Record{}, ctx, fmt.Errorf("project: load actor member: %w", err)
	}
	if actor.ID == "" {
		return member.Record{}, ctx, fmt.Errorf("project: actor member id is empty")
	}
	if strings.TrimSpace(actor.ProjectID) == "" {
		return member.Record{}, ctx, fmt.Errorf("project: actor member project id is empty")
	}
	if strings.TrimSpace(actor.LifecycleState) != member.LifecycleActive {
		return member.Record{}, ctx, fmt.Errorf("project: actor member is not active")
	}
	ctx = caller.ContextWithCaller(ctx, caller.Caller{
		UserID:    strings.TrimSpace(actor.UserID),
		MemberID:  actor.ID,
		ProjectID: types.ProjectID(strings.TrimSpace(actor.ProjectID)),
	})
	return actor, ctx, nil
}

func requireCoordinatorActor(actor member.Record, action string) error {
	if !member.IsCoordinatorType(strings.TrimSpace(actor.MemberType)) {
		return fmt.Errorf("project: action %s requires a coordinator member", action)
	}
	return nil
}

func (h Handler) registerContext(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.ContextRegistrar == nil {
		return Result{}, fmt.Errorf("project: context registrar is not configured")
	}
	token := strings.TrimSpace(call.MCPToken)
	if token == "" {
		return Result{}, fmt.Errorf("project: mcp token is required for action=register")
	}
	harnessKind := strings.TrimSpace(input.HarnessKind)
	if harnessKind == "" {
		harnessKind = strings.TrimSpace(call.HarnessKind)
	}
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(call.SessionID)
	}
	threadID := strings.TrimSpace(input.ThreadID)
	if callThreadID := strings.TrimSpace(call.ThreadID); callThreadID != "" {
		threadID = callThreadID
	}
	result, err := call.ContextRegistrar.RegisterMCPContext(ctx, RegisterContextRequest{
		Token:            token,
		BoundProjectID:   strings.TrimSpace(call.ProjectID),
		ProjectID:        input.ProjectID,
		ProjectRoot:      input.ProjectRoot,
		LocationID:       input.LocationID,
		DisplayName:      input.DisplayName,
		HarnessKind:      harnessKind,
		SessionID:        sessionID,
		ThreadID:         threadID,
		NativeSessionRef: input.NativeSessionRef,
		Model:            input.Model,
		Effort:           input.Effort,
		PermissionMode:   input.PermissionMode,
		ConfigRef:        input.ConfigRef,
	})
	if err != nil {
		return Result{}, fmt.Errorf("project: register context: %w", err)
	}
	structured := map[string]any{
		"ok":          true,
		"tool":        Name,
		"action":      "register",
		"projectId":   strings.TrimSpace(result.ProjectID),
		"projectRoot": strings.TrimSpace(result.ProjectRoot),
		"locationId":  strings.TrimSpace(result.LocationID),
		"memberId":    strings.TrimSpace(result.MemberID),
		"displayName": strings.TrimSpace(result.DisplayName),
		"memberType":  strings.TrimSpace(result.MemberType),
		"channelId":   strings.TrimSpace(result.ChannelID),
		"token":       strings.TrimSpace(result.Token),
		"url":         strings.TrimSpace(result.URL),
		"mcpServers":  append([]string(nil), result.MCPServers...),
		"guidance":    "Use display_name on project.register when the human asked you to adopt a role, e.g. \"backend engineer\" or \"frontend reviewer\". Pick names that make the graph readable.",
	}
	if strings.TrimSpace(result.SessionID) != "" {
		structured["sessionId"] = strings.TrimSpace(result.SessionID)
	}
	if strings.TrimSpace(result.ThreadID) != "" {
		structured["threadId"] = strings.TrimSpace(result.ThreadID)
	}
	if strings.TrimSpace(result.NativeSessionRef) != "" {
		structured["nativeSessionRef"] = strings.TrimSpace(result.NativeSessionRef)
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func (h Handler) memberList(ctx context.Context, call CallContext, projectID string) (Result, error) {
	if call.Members == nil {
		return Result{}, fmt.Errorf("project: member service is not configured")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Result{}, fmt.Errorf("project: session project id is required for action=member_list")
	}
	members, err := call.Members.ListMembers(ctx, member.Filter{
		ProjectID:      projectID,
		LifecycleState: member.LifecycleActive,
		Limit:          100,
	})
	if err != nil {
		return Result{}, fmt.Errorf("project: list members: %w", err)
	}
	entries := make([]memberEntry, 0, len(members))
	for _, item := range members {
		entries = append(entries, toMemberEntry(item))
	}
	structured := map[string]any{
		"ok":        true,
		"tool":      Name,
		"action":    "member_list",
		"projectId": projectID,
		"members":   entries,
		"count":     len(entries),
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func (h Handler) memberGet(ctx context.Context, call CallContext, memberID string) (Result, error) {
	if call.Members == nil {
		return Result{}, fmt.Errorf("project: member service is not configured")
	}
	memberID = strings.TrimSpace(memberID)
	if memberID == "" {
		return Result{}, fmt.Errorf("project: member_id is required for action=member_get")
	}
	loaded, err := call.Members.GetMember(ctx, member.ID(memberID))
	if err != nil {
		return Result{}, fmt.Errorf("project: get member: %w", err)
	}
	structured := map[string]any{
		"ok":     true,
		"tool":   Name,
		"action": "member_get",
		"member": toMemberEntry(loaded),
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func (h Handler) createMember(ctx context.Context, call CallContext, projectID string, input requestInput) (Result, error) {
	if call.Registrar == nil {
		return Result{}, fmt.Errorf("project: member registrar is not configured")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Result{}, fmt.Errorf("project: session project id is required for action=member_create")
	}
	result, err := call.Registrar.RegisterMember(ctx, member.Record{
		ProjectID:   projectID,
		DisplayName: input.DisplayName,
		MemberType:  member.TypeWorker,
		HarnessKind: input.HarnessKind,
		Model:       input.Model,
		Effort:      input.Effort,
	})
	if err != nil {
		return Result{}, fmt.Errorf("project: create member: %w", err)
	}
	structured := map[string]any{
		"ok":                true,
		"tool":              Name,
		"action":            "member_create",
		"projectId":         projectID,
		"member":            toMemberEntry(result.Member),
		"grantedMemberType": strings.TrimSpace(result.GrantedMemberType),
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func (h Handler) memberUpdateConfig(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.Registrar == nil {
		return Result{}, fmt.Errorf("project: member registrar is not configured")
	}
	updated, err := call.Registrar.UpdateMemberConfig(ctx, member.ID(input.MemberID), input.Model, input.Effort, input.HarnessKind)
	if err != nil {
		return Result{}, fmt.Errorf("project: update member config: %w", err)
	}
	structured := map[string]any{
		"ok":     true,
		"tool":   Name,
		"action": "member_update_config",
		"member": toMemberEntry(updated),
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func (h Handler) memberRemove(ctx context.Context, call CallContext, memberID string) (Result, error) {
	if call.Registrar == nil {
		return Result{}, fmt.Errorf("project: member registrar is not configured")
	}
	removed, err := call.Registrar.RemoveMember(ctx, member.ID(memberID))
	if err != nil {
		return Result{}, fmt.Errorf("project: remove member: %w", err)
	}
	structured := map[string]any{
		"ok":     true,
		"tool":   Name,
		"action": "member_remove",
		"member": toMemberEntry(removed),
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

type memberEntry struct {
	ID             string `json:"id"`
	ProjectID      string `json:"projectId,omitempty"`
	DisplayName    string `json:"displayName,omitempty"`
	MemberType     string `json:"memberType,omitempty"`
	LifecycleState string `json:"lifecycleState,omitempty"`
	ChannelID      string `json:"channelId,omitempty"`
}

type request struct {
	Action           string  `json:"action"`
	ProjectID        *string `json:"project_id"`
	ProjectRoot      *string `json:"project_root"`
	LocationID       *string `json:"location_id"`
	MemberID         *string `json:"member_id"`
	DisplayName      *string `json:"display_name"`
	HarnessKind      *string `json:"harness_kind"`
	SessionID        *string `json:"session_id"`
	ThreadID         *string `json:"thread_id"`
	NativeSessionRef *string `json:"native_session_ref"`
	Model            *string `json:"model"`
	Effort           *string `json:"effort"`
	PermissionMode   *string `json:"permission_mode"`
	ConfigRef        *string `json:"config_ref"`
}

func decode(args json.RawMessage) (requestInput, error) {
	if err := validateActionFields(args); err != nil {
		return requestInput{}, err
	}
	var raw request
	decoder := json.NewDecoder(bytes.NewReader(args))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return requestInput{}, fmt.Errorf("project: invalid arguments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return requestInput{}, fmt.Errorf("project: invalid arguments: trailing JSON")
	}
	action := strings.TrimSpace(strings.ToLower(raw.Action))
	if action == "" {
		return requestInput{}, fmt.Errorf("project: action is required")
	}
	if !containsAction(action) {
		return requestInput{}, fmt.Errorf("project: unsupported action %q", action)
	}
	input := requestInput{
		Action:           action,
		ProjectID:        strings.TrimSpace(ptrString(raw.ProjectID)),
		ProjectRoot:      strings.TrimSpace(ptrString(raw.ProjectRoot)),
		LocationID:       strings.TrimSpace(ptrString(raw.LocationID)),
		MemberID:         strings.TrimSpace(ptrString(raw.MemberID)),
		DisplayName:      strings.TrimSpace(ptrString(raw.DisplayName)),
		HarnessKind:      strings.TrimSpace(ptrString(raw.HarnessKind)),
		SessionID:        strings.TrimSpace(ptrString(raw.SessionID)),
		ThreadID:         strings.TrimSpace(ptrString(raw.ThreadID)),
		NativeSessionRef: strings.TrimSpace(ptrString(raw.NativeSessionRef)),
		Model:            strings.TrimSpace(ptrString(raw.Model)),
		Effort:           strings.TrimSpace(ptrString(raw.Effort)),
		PermissionMode:   strings.TrimSpace(ptrString(raw.PermissionMode)),
		ConfigRef:        strings.TrimSpace(ptrString(raw.ConfigRef)),
	}
	switch action {
	case "register":
		if input.ProjectID == "" && input.ProjectRoot == "" {
			return requestInput{}, fmt.Errorf("project: project_root or project_id is required for action=register")
		}
	case "member_create":
		if err := requireRuntimeConfig(input, "member_create"); err != nil {
			return requestInput{}, err
		}
	case "member_get", "member_remove":
		if input.MemberID == "" {
			return requestInput{}, fmt.Errorf("project: member_id is required for action=%s", action)
		}
	case "member_update_config":
		if input.MemberID == "" {
			return requestInput{}, fmt.Errorf("project: member_id is required for action=member_update_config")
		}
		if err := requireRuntimeConfig(input, "member_update_config"); err != nil {
			return requestInput{}, err
		}
	}
	return input, nil
}

func requireRuntimeConfig(input requestInput, action string) error {
	if input.HarnessKind == "" {
		return fmt.Errorf("project: harness_kind is required for action=%s", action)
	}
	if input.Model == "" {
		return fmt.Errorf("project: model is required for action=%s", action)
	}
	if input.Effort == "" {
		return fmt.Errorf("project: effort is required for action=%s", action)
	}
	return nil
}

func validateActionFields(args json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return fmt.Errorf("project: invalid arguments: %w", err)
	}
	actionRaw, ok := fields["action"]
	if !ok || isJSONNull(actionRaw) {
		return fmt.Errorf("project: action is required")
	}
	var action string
	if err := json.Unmarshal(actionRaw, &action); err != nil {
		return fmt.Errorf("project: action must be a string")
	}
	action = strings.TrimSpace(strings.ToLower(action))
	if action == "" {
		return fmt.Errorf("project: action is required")
	}
	allowed, ok := fieldsByAction[action]
	if !ok {
		return fmt.Errorf("project: unsupported action %q", action)
	}
	for field, raw := range fields {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("project: field %q is not valid for action %q", field, action)
		}
		if isJSONNull(raw) {
			return fmt.Errorf("project: field %q must be omitted instead of null", field)
		}
	}
	return nil
}

var fieldsByAction = map[string]map[string]struct{}{
	"register":             fieldSet("action", "project_id", "project_root", "location_id", "display_name", "harness_kind", "session_id", "thread_id", "native_session_ref", "model", "effort", "permission_mode", "config_ref"),
	"member_create":        fieldSet("action", "display_name", "harness_kind", "model", "effort"),
	"member_get":           fieldSet("action", "member_id"),
	"member_list":          fieldSet("action"),
	"member_update_config": fieldSet("action", "member_id", "harness_kind", "model", "effort"),
	"member_remove":        fieldSet("action", "member_id"),
}

func fieldSet(fields ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		out[field] = struct{}{}
	}
	return out
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

type requestInput struct {
	Action           string
	ProjectID        string
	ProjectRoot      string
	LocationID       string
	MemberID         string
	DisplayName      string
	HarnessKind      string
	SessionID        string
	ThreadID         string
	NativeSessionRef string
	Model            string
	Effort           string
	PermissionMode   string
	ConfigRef        string
}

func mustSchema(actions []string) json.RawMessage {
	body, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{"type": "string", "enum": actions},
			"project_id": map[string]any{
				"type":        "string",
				"description": "Existing project ID for action=register. Omit when providing project_root.",
			},
			"project_root": map[string]any{
				"type":        "string",
				"description": "Working directory to create or reuse a project for action=register.",
			},
			"location_id": map[string]any{
				"type":        "string",
				"description": "Optional location ID for action=register. Defaults to local.",
			},
			"member_id": map[string]any{
				"type":        "string",
				"description": "Required for member_get, member_update_config, and member_remove.",
			},
			"display_name": map[string]any{
				"type":        "string",
				"description": "Optional display label for action=register and action=member_create. For register, use a role-based name the human can recognize in the graph, such as backend engineer, frontend reviewer, or research analyst.",
			},
			"harness_kind": map[string]any{
				"type":        "string",
				"description": "Runtime harness kind for register, member_create, and member_update_config, such as codex or claude-cli.",
			},
			"session_id": map[string]any{
				"type":        "string",
				"description": "Optional stable harness session id for action=register. When supplied, Agen8 creates or reuses a member for that session.",
			},
			"thread_id": map[string]any{
				"type":        "string",
				"description": "Optional stable harness thread id for action=register. When supplied, Agen8 creates or reuses a member for that thread.",
			},
			"native_session_ref": map[string]any{
				"type":        "string",
				"description": "Optional live native harness session reference for action=register when it differs from the stable session_id.",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Required runtime model for member_create and member_update_config.",
			},
			"effort": map[string]any{
				"type":        "string",
				"description": "Reasoning effort for register, member_create, and member_update_config. Register defaults to medium.",
			},
			"permission_mode": map[string]any{
				"type":        "string",
				"description": "Optional runtime permission mode for action=register. Defaults to the harness catalog default.",
			},
			"config_ref": map[string]any{
				"type":        "string",
				"description": "Optional runtime config reference for action=register when the selected permission mode requires it.",
			},
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	})
	if err != nil {
		panic(fmt.Sprintf("project schema encode: %v", err))
	}
	return body
}

func containsAction(action string) bool {
	for _, allowed := range allActions {
		if action == allowed {
			return true
		}
	}
	return false
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func toMemberEntry(item member.Record) memberEntry {
	return memberEntry{
		ID:             strings.TrimSpace(string(item.ID)),
		ProjectID:      strings.TrimSpace(item.ProjectID),
		DisplayName:    strings.TrimSpace(item.DisplayName),
		MemberType:     strings.TrimSpace(item.MemberType),
		LifecycleState: strings.TrimSpace(item.LifecycleState),
		ChannelID:      strings.TrimSpace(string(item.ChannelID)),
	}
}

func encodeText(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("project: encode structured response: %w", err)
	}
	return string(encoded), nil
}
