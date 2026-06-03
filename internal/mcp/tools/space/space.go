package space

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	spaceapp "github.com/tinoosan/agen8-mcp-server/internal/services/space/app"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

const Name = "space"
const Description = "[COORDINATION] Space gateway for workspace registration, spaces, and member roster CRUD. Call action=register first when the MCP session is not yet bound to a project/space/member."

var allActions = []string{"register", "list", "create", "member_create", "member_get", "member_list", "member_update_config", "member_remove"}

func BootstrapActionAllowed(action string) bool {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "register", "list", "create":
		return true
	default:
		return false
	}
}

type SpaceService interface {
	Get(ctx context.Context, id spacedomain.SpaceID) (spacedomain.SpaceRecord, error)
	List(ctx context.Context, filter spacedomain.SpaceFilter) ([]spacedomain.SpaceRecord, error)
}

type SetupService interface {
	Create(ctx context.Context, space spacedomain.SpaceRecord) (spacedomain.SpaceRecord, error)
}

type MemberService interface {
	GetMember(ctx context.Context, id member.ID) (member.Record, error)
	ListMembers(ctx context.Context, filter member.Filter) ([]member.Record, error)
}

type MemberRegistrar interface {
	RegisterMember(ctx context.Context, rosterMember member.Record) (spaceapp.RegisterMemberResult, error)
	UpdateMemberConfig(ctx context.Context, id member.ID, model, effort, harnessKind string, permissionFields ...string) (member.Record, error)
	RemoveMember(ctx context.Context, id member.ID) (member.Record, error)
}

type ContextRegistrar interface {
	RegisterMCPContext(ctx context.Context, req RegisterContextRequest) (RegisterContextResult, error)
}

type RegisterContextRequest struct {
	Token          string
	ProjectID      string
	ProjectRoot    string
	LocationID     string
	SpaceID        string
	DisplayName    string
	HarnessKind    string
	SessionID      string
	ThreadID       string
	Model          string
	Effort         string
	PermissionMode string
	ConfigRef      string
}

type RegisterContextResult struct {
	ProjectID        string
	ProjectRoot      string
	LocationID       string
	SpaceID          string
	MemberID         string
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
	Spaces           SpaceService
	Members          MemberService
	Registrar        MemberRegistrar
	ContextRegistrar ContextRegistrar
	SpaceSetup       SetupService
	MCPToken         string
	UserID           string
	HarnessKind      string
	ProjectID        string
	SpaceID          string
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
	if input.SpaceID == "" {
		input.SpaceID = strings.TrimSpace(call.SpaceID)
	}
	if input.Action == "register" {
		return h.registerContext(ctx, call, input)
	}
	ctx = contextWithSetupCaller(ctx, call)
	if input.Action == "list" {
		return h.list(ctx, call, input)
	}
	if input.Action == "create" {
		return h.createSpace(ctx, call, input)
	}
	actor, err := h.resolveActor(ctx, call)
	if err != nil {
		return Result{}, err
	}
	ctx = caller.ContextWithCaller(ctx, caller.Caller{
		UserID:   strings.TrimSpace(actor.UserID),
		MemberID: actor.ID,
		SpaceID:  spacedomain.SpaceID(actor.SpaceID),
	})
	if input.Action == "member_create" || input.Action == "member_update_config" || input.Action == "member_remove" {
		if err := requireCoordinatorActor(actor, input.Action); err != nil {
			return Result{}, err
		}
	}
	switch input.Action {
	case "member_list":
		return h.memberList(ctx, call, input.SpaceID)
	case "member_get":
		return h.memberGet(ctx, call, input.MemberID)
	case "member_create":
		return h.createMember(ctx, call, input)
	case "member_update_config":
		return h.memberUpdateConfig(ctx, call, input)
	case "member_remove":
		return h.memberRemove(ctx, call, input.MemberID)
	default:
		return Result{}, fmt.Errorf("space: unsupported action %q", input.Action)
	}
}

func contextWithSetupCaller(ctx context.Context, call CallContext) context.Context {
	userID := strings.TrimSpace(call.UserID)
	if userID != "" {
		return caller.ContextWithCaller(ctx, caller.Caller{
			UserID:   userID,
			MemberID: member.ID(strings.TrimSpace(call.ActorMemberID)),
			SpaceID:  spacedomain.SpaceID(strings.TrimSpace(call.SpaceID)),
		})
	}
	return contextWithSessionActor(ctx, call.ActorMemberID, call.SpaceID)
}

func contextWithSessionActor(ctx context.Context, actorMemberID, spaceID string) context.Context {
	actorMemberID = strings.TrimSpace(actorMemberID)
	spaceID = strings.TrimSpace(spaceID)
	if actorMemberID == "" && spaceID == "" {
		return ctx
	}
	return caller.ContextWithCaller(ctx, caller.Caller{
		MemberID: member.ID(actorMemberID),
		SpaceID:  spacedomain.SpaceID(spaceID),
	})
}

func (h Handler) resolveActor(ctx context.Context, call CallContext) (member.Record, error) {
	if call.Members == nil {
		return member.Record{}, fmt.Errorf("space: member service is not configured")
	}
	memberID := strings.TrimSpace(call.ActorMemberID)
	if memberID == "" {
		return member.Record{}, fmt.Errorf("space: actor member id is required")
	}
	actor, err := call.Members.GetMember(ctx, member.ID(memberID))
	if err != nil {
		return member.Record{}, fmt.Errorf("space: load actor member: %w", err)
	}
	if actor.ID == "" {
		return member.Record{}, fmt.Errorf("space: actor member id is empty")
	}
	if actor.SpaceID == "" {
		return member.Record{}, fmt.Errorf("space: actor member space id is empty")
	}
	if strings.TrimSpace(actor.LifecycleState) != member.LifecycleActive {
		return member.Record{}, fmt.Errorf("space: actor member is not active")
	}
	return actor, nil
}

func requireCoordinatorActor(actor member.Record, action string) error {
	if !member.IsCoordinatorType(strings.TrimSpace(actor.MemberType)) {
		return fmt.Errorf("space: action %s requires a coordinator member", action)
	}
	return nil
}

func (h Handler) list(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.Spaces == nil {
		return Result{}, fmt.Errorf("space: space service is not configured")
	}
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		projectID = strings.TrimSpace(call.ProjectID)
	}
	spaces, err := call.Spaces.List(ctx, spacedomain.SpaceFilter{
		ProjectID: projectID,
		Status:    spacedomain.SpaceStatusOpen,
		Limit:     100,
	})
	if err != nil {
		return Result{}, fmt.Errorf("space: list spaces: %w", err)
	}
	entries := make([]spaceEntry, 0, len(spaces))
	for _, item := range spaces {
		entries = append(entries, spaceEntry{
			ID:        strings.TrimSpace(string(item.ID)),
			ProjectID: strings.TrimSpace(string(item.ProjectID)),
			Title:     strings.TrimSpace(item.Title),
			Status:    strings.TrimSpace(item.Status),
			PlanMode:  strings.TrimSpace(item.PlanMode),
		})
	}
	structured := map[string]any{
		"ok":     true,
		"tool":   Name,
		"action": "list",
		"spaces": entries,
		"count":  len(entries),
	}
	if projectID != "" {
		structured["projectId"] = projectID
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func (h Handler) createSpace(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.SpaceSetup == nil {
		return Result{}, fmt.Errorf("space: setup service is not configured")
	}
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		projectID = strings.TrimSpace(call.ProjectID)
	}
	if projectID == "" {
		return Result{}, fmt.Errorf("space: project_id is required for action=create")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "MCP Work Context"
	}
	spaceID := strings.TrimSpace(input.SpaceID)
	if spaceID == "" {
		spaceID = "space-" + stableShortID(projectID+"\x00"+title)
	}
	created, err := call.SpaceSetup.Create(ctx, spacedomain.SpaceRecord{
		ID:        spacedomain.SpaceID(spaceID),
		ProjectID: projectID,
		Title:     title,
		Status:    spacedomain.SpaceStatusOpen,
	})
	if err != nil {
		return Result{}, fmt.Errorf("space: create space: %w", err)
	}
	structured := map[string]any{
		"ok":     true,
		"tool":   Name,
		"action": "create",
		"space":  toSpaceEntry(created),
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func (h Handler) registerContext(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.ContextRegistrar == nil {
		return Result{}, fmt.Errorf("space: context registrar is not configured")
	}
	token := strings.TrimSpace(call.MCPToken)
	if token == "" {
		return Result{}, fmt.Errorf("space: mcp token is required for action=register")
	}
	harnessKind := strings.TrimSpace(input.HarnessKind)
	if harnessKind == "" {
		harnessKind = strings.TrimSpace(call.HarnessKind)
	}
	sessionID := strings.TrimSpace(call.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(input.SessionID)
	}
	threadID := strings.TrimSpace(call.ThreadID)
	if threadID == "" {
		threadID = strings.TrimSpace(input.ThreadID)
	}
	result, err := call.ContextRegistrar.RegisterMCPContext(ctx, RegisterContextRequest{
		Token:          token,
		ProjectID:      input.ProjectID,
		ProjectRoot:    input.ProjectRoot,
		LocationID:     input.LocationID,
		SpaceID:        input.SpaceID,
		DisplayName:    input.DisplayName,
		HarnessKind:    harnessKind,
		SessionID:      sessionID,
		ThreadID:       threadID,
		Model:          input.Model,
		Effort:         input.Effort,
		PermissionMode: input.PermissionMode,
		ConfigRef:      input.ConfigRef,
	})
	if err != nil {
		return Result{}, fmt.Errorf("space: register context: %w", err)
	}
	structured := map[string]any{
		"ok":          true,
		"tool":        Name,
		"action":      "register",
		"projectId":   strings.TrimSpace(result.ProjectID),
		"projectRoot": strings.TrimSpace(result.ProjectRoot),
		"locationId":  strings.TrimSpace(result.LocationID),
		"spaceId":     strings.TrimSpace(result.SpaceID),
		"memberId":    strings.TrimSpace(result.MemberID),
		"memberType":  strings.TrimSpace(result.MemberType),
		"channelId":   strings.TrimSpace(result.ChannelID),
		"token":       strings.TrimSpace(result.Token),
		"url":         strings.TrimSpace(result.URL),
		"mcpServers":  append([]string(nil), result.MCPServers...),
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

func (h Handler) memberList(ctx context.Context, call CallContext, spaceID string) (Result, error) {
	if call.Spaces == nil {
		return Result{}, fmt.Errorf("space: space service is not configured")
	}
	if call.Members == nil {
		return Result{}, fmt.Errorf("space: member service is not configured")
	}
	spaceID = strings.TrimSpace(spaceID)
	if spaceID == "" {
		return Result{}, fmt.Errorf("space: session space id is required when space_id is omitted")
	}
	loaded, err := call.Spaces.Get(ctx, spacedomain.SpaceID(spaceID))
	if err != nil {
		return Result{}, fmt.Errorf("space: load space: %w", err)
	}
	members, err := call.Members.ListMembers(ctx, member.Filter{
		SpaceID:        spaceID,
		LifecycleState: member.LifecycleActive,
		Limit:          100,
	})
	if err != nil {
		return Result{}, fmt.Errorf("space: list members: %w", err)
	}
	entries := make([]memberEntry, 0, len(members))
	for _, item := range members {
		entries = append(entries, toMemberEntry(item))
	}
	structured := map[string]any{
		"ok":      true,
		"tool":    Name,
		"action":  "member_list",
		"space":   toSpaceEntry(loaded),
		"members": entries,
		"count":   len(entries),
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func (h Handler) memberGet(ctx context.Context, call CallContext, memberID string) (Result, error) {
	if call.Members == nil {
		return Result{}, fmt.Errorf("space: member service is not configured")
	}
	memberID = strings.TrimSpace(memberID)
	if memberID == "" {
		return Result{}, fmt.Errorf("space: member_id is required for action=member_get")
	}
	loaded, err := call.Members.GetMember(ctx, member.ID(memberID))
	if err != nil {
		return Result{}, fmt.Errorf("space: get member: %w", err)
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

func (h Handler) createMember(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.Spaces == nil {
		return Result{}, fmt.Errorf("space: space service is not configured")
	}
	if call.Registrar == nil {
		return Result{}, fmt.Errorf("space: member registrar is not configured")
	}
	spaceID := strings.TrimSpace(input.SpaceID)
	if spaceID == "" {
		return Result{}, fmt.Errorf("space: session space id is required when space_id is omitted")
	}
	loaded, err := call.Spaces.Get(ctx, spacedomain.SpaceID(spaceID))
	if err != nil {
		return Result{}, fmt.Errorf("space: load space: %w", err)
	}
	result, err := call.Registrar.RegisterMember(ctx, member.Record{
		SpaceID:     spaceID,
		ProjectID:   strings.TrimSpace(call.ProjectID),
		DisplayName: input.DisplayName,
		MemberType:  member.TypeWorker,
		HarnessKind: input.HarnessKind,
		Model:       input.Model,
		Effort:      input.Effort,
	})
	if err != nil {
		return Result{}, fmt.Errorf("space: create member: %w", err)
	}
	entry := toMemberEntry(result.Member)
	structured := map[string]any{
		"ok":                true,
		"tool":              Name,
		"action":            "member_create",
		"space":             toSpaceEntry(loaded),
		"member":            entry,
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
		return Result{}, fmt.Errorf("space: member registrar is not configured")
	}
	updated, err := call.Registrar.UpdateMemberConfig(ctx, member.ID(input.MemberID), input.Model, input.Effort, input.HarnessKind)
	if err != nil {
		return Result{}, fmt.Errorf("space: update member config: %w", err)
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
		return Result{}, fmt.Errorf("space: member registrar is not configured")
	}
	removed, err := call.Registrar.RemoveMember(ctx, member.ID(memberID))
	if err != nil {
		return Result{}, fmt.Errorf("space: remove member: %w", err)
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

type spaceEntry struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId,omitempty"`
	Title     string `json:"title,omitempty"`
	Status    string `json:"status,omitempty"`
	PlanMode  string `json:"planMode,omitempty"`
}

type memberEntry struct {
	ID             string `json:"id"`
	DisplayName    string `json:"displayName,omitempty"`
	MemberType     string `json:"memberType,omitempty"`
	LifecycleState string `json:"lifecycleState,omitempty"`
	ChannelID      string `json:"channelId,omitempty"`
}

type request struct {
	Action         string  `json:"action"`
	ProjectID      *string `json:"project_id"`
	ProjectRoot    *string `json:"project_root"`
	LocationID     *string `json:"location_id"`
	SpaceID        *string `json:"space_id"`
	MemberID       *string `json:"member_id"`
	DisplayName    *string `json:"display_name"`
	Title          *string `json:"title"`
	HarnessKind    *string `json:"harness_kind"`
	SessionID      *string `json:"session_id"`
	ThreadID       *string `json:"thread_id"`
	Model          *string `json:"model"`
	Effort         *string `json:"effort"`
	PermissionMode *string `json:"permission_mode"`
	ConfigRef      *string `json:"config_ref"`
}

func decode(args json.RawMessage) (requestInput, error) {
	if err := validateActionFields(args); err != nil {
		return requestInput{}, err
	}
	var raw request
	decoder := json.NewDecoder(bytes.NewReader(args))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return requestInput{}, fmt.Errorf("space: invalid arguments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return requestInput{}, fmt.Errorf("space: invalid arguments: trailing JSON")
	}
	action := strings.TrimSpace(strings.ToLower(raw.Action))
	if action == "" {
		return requestInput{}, fmt.Errorf("space: action is required")
	}
	if !containsAction(action) {
		return requestInput{}, fmt.Errorf("space: unsupported action %q", action)
	}
	input := requestInput{
		Action:         action,
		ProjectID:      strings.TrimSpace(ptrString(raw.ProjectID)),
		ProjectRoot:    strings.TrimSpace(ptrString(raw.ProjectRoot)),
		LocationID:     strings.TrimSpace(ptrString(raw.LocationID)),
		SpaceID:        strings.TrimSpace(ptrString(raw.SpaceID)),
		MemberID:       strings.TrimSpace(ptrString(raw.MemberID)),
		DisplayName:    strings.TrimSpace(ptrString(raw.DisplayName)),
		Title:          strings.TrimSpace(ptrString(raw.Title)),
		HarnessKind:    strings.TrimSpace(ptrString(raw.HarnessKind)),
		SessionID:      strings.TrimSpace(ptrString(raw.SessionID)),
		ThreadID:       strings.TrimSpace(ptrString(raw.ThreadID)),
		Model:          strings.TrimSpace(ptrString(raw.Model)),
		Effort:         strings.TrimSpace(ptrString(raw.Effort)),
		PermissionMode: strings.TrimSpace(ptrString(raw.PermissionMode)),
		ConfigRef:      strings.TrimSpace(ptrString(raw.ConfigRef)),
	}
	switch action {
	case "register":
		if input.ProjectID == "" && input.ProjectRoot == "" {
			return requestInput{}, fmt.Errorf("space: project_root or project_id is required for action=register")
		}
	case "create":
		if input.ProjectID == "" {
			return requestInput{}, fmt.Errorf("space: project_id is required for action=create")
		}
	case "member_create":
		if err := requireRuntimeConfig(input, "member_create"); err != nil {
			return requestInput{}, err
		}
	case "member_get", "member_remove":
		if input.MemberID == "" {
			return requestInput{}, fmt.Errorf("space: member_id is required for action=%s", action)
		}
	case "member_update_config":
		if input.MemberID == "" {
			return requestInput{}, fmt.Errorf("space: member_id is required for action=member_update_config")
		}
		if err := requireRuntimeConfig(input, "member_update_config"); err != nil {
			return requestInput{}, err
		}
	}
	return input, nil
}

func requireRuntimeConfig(input requestInput, action string) error {
	if input.HarnessKind == "" {
		return fmt.Errorf("space: harness_kind is required for action=%s", action)
	}
	if input.Model == "" {
		return fmt.Errorf("space: model is required for action=%s", action)
	}
	if input.Effort == "" {
		return fmt.Errorf("space: effort is required for action=%s", action)
	}
	return nil
}

func validateActionFields(args json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return fmt.Errorf("space: invalid arguments: %w", err)
	}
	actionRaw, ok := fields["action"]
	if !ok || isJSONNull(actionRaw) {
		return fmt.Errorf("space: action is required")
	}
	var action string
	if err := json.Unmarshal(actionRaw, &action); err != nil {
		return fmt.Errorf("space: action must be a string")
	}
	action = strings.TrimSpace(strings.ToLower(action))
	if action == "" {
		return fmt.Errorf("space: action is required")
	}
	allowed, ok := fieldsByAction[action]
	if !ok {
		return fmt.Errorf("space: unsupported action %q", action)
	}
	for field, raw := range fields {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("space: field %q is not valid for action %q", field, action)
		}
		if isJSONNull(raw) {
			return fmt.Errorf("space: field %q must be omitted instead of null", field)
		}
	}
	return nil
}

var fieldsByAction = map[string]map[string]struct{}{
	"register":             fieldSet("action", "project_id", "project_root", "location_id", "space_id", "display_name", "harness_kind", "session_id", "thread_id", "model", "effort", "permission_mode", "config_ref"),
	"list":                 fieldSet("action", "project_id"),
	"create":               fieldSet("action", "project_id", "space_id", "title"),
	"member_create":        fieldSet("action", "space_id", "display_name", "harness_kind", "model", "effort"),
	"member_get":           fieldSet("action", "member_id"),
	"member_list":          fieldSet("action", "space_id"),
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
	Action         string
	ProjectID      string
	ProjectRoot    string
	LocationID     string
	SpaceID        string
	MemberID       string
	DisplayName    string
	Title          string
	HarnessKind    string
	SessionID      string
	ThreadID       string
	Model          string
	Effort         string
	PermissionMode string
	ConfigRef      string
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
			"space_id": map[string]any{"type": "string", "description": "Space ID for action=member_list, action=member_create, or action=create. Omit to use the caller's current space where supported."},
			"title":    map[string]any{"type": "string", "description": "Space title for action=create."},
			"member_id": map[string]any{
				"type":        "string",
				"description": "Required for member_get, member_update_config, and member_remove.",
			},
			"display_name": map[string]any{
				"type":        "string",
				"description": "Optional display label for action=member_create.",
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
		panic(fmt.Sprintf("space schema encode: %v", err))
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

func stableShortID(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:8])
}

func toMemberEntry(item member.Record) memberEntry {
	return memberEntry{
		ID:             strings.TrimSpace(string(item.ID)),
		DisplayName:    strings.TrimSpace(item.DisplayName),
		MemberType:     strings.TrimSpace(item.MemberType),
		LifecycleState: strings.TrimSpace(item.LifecycleState),
		ChannelID:      strings.TrimSpace(string(item.ChannelID)),
	}
}

func toSpaceEntry(space spacedomain.SpaceRecord) spaceEntry {
	return spaceEntry{
		ID:        strings.TrimSpace(string(space.ID)),
		ProjectID: strings.TrimSpace(string(space.ProjectID)),
		Title:     strings.TrimSpace(space.Title),
		Status:    strings.TrimSpace(space.Status),
		PlanMode:  strings.TrimSpace(space.PlanMode),
	}
}

func encodeText(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("space: encode structured response: %w", err)
	}
	return string(encoded), nil
}
