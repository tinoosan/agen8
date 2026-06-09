package project

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	projectapp "github.com/tinoosan/agen8/internal/services/project/app"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
)

const Name = "project"
const Description = "[COORDINATION] Project gateway for workspace registration and member roster CRUD. Call action=register first when the MCP session is not yet bound to a project/member."

var allActions = []string{"register", "member_create", "member_get", "member_list", "member_update", "member_remove"}

func BootstrapActionAllowed(action string) bool {
	return strings.TrimSpace(strings.ToLower(action)) == "register"
}

type MemberService interface {
	GetMember(ctx context.Context, id member.ID) (member.Record, error)
	ListMembers(ctx context.Context, filter member.Filter) ([]member.Record, error)
}

type MemberRegistrar interface {
	RegisterMember(ctx context.Context, rosterMember member.Record) (projectapp.RegisterMemberResult, error)
	UpdateMember(ctx context.Context, id member.ID, displayName string) (member.Record, error)
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
	ProjectID         string
	ProjectRoot       string
	LocationID        string
	MemberID          string
	DisplayName       string
	MemberType        string
	ChannelID         string
	SessionID         string
	ThreadID          string
	NativeSessionRef  string
	Token             string
	URL               string
	MCPServers        []string
	AlreadyRegistered bool
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
	if input.Action == "member_create" || input.Action == "member_update" || input.Action == "member_remove" {
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
	case "member_update":
		return h.memberUpdate(ctx, call, input)
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
	if strings.TrimSpace(actor.ProjectID) == "" {
		return fmt.Errorf("project: action %s requires an active project member", action)
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
	harnessKind := defaultHarnessKind(call)
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
		"guidance":    registerGuidance(result),
	}
	if result.AlreadyRegistered {
		structured["alreadyRegistered"] = true
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
		MemberType:  member.TypeCoordinator,
		HarnessKind: defaultHarnessKind(call),
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

func (h Handler) memberUpdate(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.Registrar == nil {
		return Result{}, fmt.Errorf("project: member registrar is not configured")
	}
	updated, err := call.Registrar.UpdateMember(ctx, member.ID(input.MemberID), input.DisplayName)
	if err != nil {
		return Result{}, fmt.Errorf("project: update member: %w", err)
	}
	structured := map[string]any{
		"ok":     true,
		"tool":   Name,
		"action": "member_update",
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
	ID               string     `json:"id"`
	UserID           string     `json:"userId,omitempty"`
	ProjectID        string     `json:"projectId,omitempty"`
	NativeSessionRef string     `json:"nativeSessionRef,omitempty"`
	ChannelID        string     `json:"channelId,omitempty"`
	DisplayName      string     `json:"displayName,omitempty"`
	MemberType       string     `json:"memberType,omitempty"`
	HarnessKind      string     `json:"harnessKind,omitempty"`
	LifecycleState   string     `json:"lifecycleState,omitempty"`
	RegisteredAt     time.Time  `json:"registeredAt,omitempty"`
	UpdatedAt        time.Time  `json:"updatedAt,omitempty"`
	LastSeenAt       *time.Time `json:"lastSeenAt,omitempty"`
}

type request struct {
	Action           string  `json:"action"`
	ProjectID        *string `json:"project_id"`
	ProjectRoot      *string `json:"project_root"`
	LocationID       *string `json:"location_id"`
	MemberID         *string `json:"member_id"`
	DisplayName      *string `json:"display_name"`
	SessionID        *string `json:"session_id"`
	ThreadID         *string `json:"thread_id"`
	NativeSessionRef *string `json:"native_session_ref"`
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
		SessionID:        strings.TrimSpace(ptrString(raw.SessionID)),
		ThreadID:         strings.TrimSpace(ptrString(raw.ThreadID)),
		NativeSessionRef: strings.TrimSpace(ptrString(raw.NativeSessionRef)),
	}
	switch action {
	case "register":
		if input.ProjectID == "" && input.ProjectRoot == "" {
			return requestInput{}, fmt.Errorf("project: project_root or project_id is required for action=register")
		}
	case "member_create":
		if input.DisplayName == "" {
			return requestInput{}, fmt.Errorf("project: display_name is required for action=member_create")
		}
	case "member_get", "member_remove":
		if input.MemberID == "" {
			return requestInput{}, fmt.Errorf("project: member_id is required for action=%s", action)
		}
	case "member_update":
		if input.MemberID == "" {
			return requestInput{}, fmt.Errorf("project: member_id is required for action=member_update")
		}
		if input.DisplayName == "" {
			return requestInput{}, fmt.Errorf("project: display_name is required for action=member_update")
		}
	}
	return input, nil
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
	"register":      fieldSet("action", "project_id", "project_root", "location_id", "display_name", "session_id", "thread_id", "native_session_ref"),
	"member_create": fieldSet("action", "display_name"),
	"member_get":    fieldSet("action", "member_id"),
	"member_list":   fieldSet("action"),
	"member_update": fieldSet("action", "member_id", "display_name"),
	"member_remove": fieldSet("action", "member_id"),
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
	SessionID        string
	ThreadID         string
	NativeSessionRef string
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
				"description": "Required for member_get, member_update, and member_remove.",
			},
			"display_name": map[string]any{
				"type":        "string",
				"description": "Display label for register, member_create, and member_update. Use a recognizable name such as 'Atlas (Backend Engineer)'.",
			},
			"session_id": map[string]any{
				"type":        "string",
				"description": "Optional stable harness session id for action=register. Most clients should omit this; Agen8 reads native session metadata when available.",
			},
			"thread_id": map[string]any{
				"type":        "string",
				"description": "Optional stable harness thread id for action=register. Most clients should omit this; Agen8 reads native session metadata when available.",
			},
			"native_session_ref": map[string]any{
				"type":        "string",
				"description": "Optional live native harness session reference for action=register. Most clients should omit this; Agen8 reads native session metadata when available.",
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
		ID:               strings.TrimSpace(string(item.ID)),
		UserID:           strings.TrimSpace(item.UserID),
		ProjectID:        strings.TrimSpace(item.ProjectID),
		NativeSessionRef: strings.TrimSpace(item.NativeSessionRef),
		ChannelID:        strings.TrimSpace(item.ChannelID),
		DisplayName:      strings.TrimSpace(item.DisplayName),
		MemberType:       strings.TrimSpace(item.MemberType),
		HarnessKind:      strings.TrimSpace(item.HarnessKind),
		LifecycleState:   strings.TrimSpace(item.LifecycleState),
		RegisteredAt:     item.RegisteredAt,
		UpdatedAt:        item.UpdatedAt,
		LastSeenAt:       item.LastSeenAt,
	}
}

func registerGuidance(result RegisterContextResult) string {
	name := strings.TrimSpace(result.DisplayName)
	memberID := strings.TrimSpace(result.MemberID)
	if result.AlreadyRegistered {
		if name == "" {
			name = "this member"
		}
		return fmt.Sprintf("You are already registered as %s (%s). Do not call project.register again for this session; use action=member_update with member_id and display_name to rename this registration.", name, memberID)
	}
	return "You are registered. Use display_name on project.register only for the name the human should see in the graph. Agen8 derives session, harness, model, and runtime details."
}

// defaultHarnessKind returns the harness the daemon detected for this session
// (see mcp.HarnessFromJSONRPCBody) and falls back to "unknown" when no signal
// identified it. Honest-MVP: we never label an undetected harness with a made-up
// value like "agent" — "unknown" says exactly what we know.
func defaultHarnessKind(call CallContext) string {
	if harnessKind := strings.TrimSpace(call.HarnessKind); harnessKind != "" {
		return harnessKind
	}
	return "unknown"
}

func encodeText(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("project: encode structured response: %w", err)
	}
	return string(encoded), nil
}
