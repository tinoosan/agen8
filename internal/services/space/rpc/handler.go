package rpc

import (
	"context"
	"strings"

	"github.com/google/uuid"
	spaceapp "github.com/tinoosan/agen8-mcp-server/internal/services/space/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type Handler struct {
	svc *spaceapp.Service
}

func NewHandler(svc *spaceapp.Service) *Handler {
	if svc == nil {
		panic("space RPC handler requires space service")
	}
	return &Handler{svc: svc}
}

func (h *Handler) SpaceCreate(ctx context.Context, p SpaceCreateParams) (SpaceCreateResult, error) {
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return SpaceCreateResult{}, invalidParams("projectId is required")
	}

	spaceID := strings.TrimSpace(p.SpaceID)
	if spaceID == "" {
		spaceID = "space-" + uuid.NewString()
	}

	space := domain.SpaceRecord{
		ID:        domain.SpaceID(spaceID),
		ProjectID: projectID,
		Title:     strings.TrimSpace(p.Title),
		PlanMode:  strings.TrimSpace(p.PlanMode),
		Status:    domain.SpaceStatusOpen,
	}

	created, err := h.svc.Create(ctx, space)
	if err != nil {
		return SpaceCreateResult{}, err
	}
	return SpaceCreateResult{Space: NewSpaceView(created)}, nil
}

func (h *Handler) SpaceGet(ctx context.Context, p SpaceGetParams) (SpaceGetResult, error) {
	id, err := requireSpaceID(p.SpaceID)
	if err != nil {
		return SpaceGetResult{}, err
	}

	space, err := h.svc.Get(ctx, id)
	if err != nil {
		return SpaceGetResult{}, err
	}
	return SpaceGetResult{Space: NewSpaceView(space)}, nil
}

func (h *Handler) SpaceList(ctx context.Context, p SpaceListParams) (SpaceListResult, error) {
	if id := strings.TrimSpace(p.SpaceID); id != "" {
		space, err := h.svc.Get(ctx, domain.SpaceID(id))
		if err != nil {
			return SpaceListResult{}, err
		}
		return SpaceListResult{Spaces: []SpaceView{NewSpaceView(space)}, TotalCount: 1}, nil
	}

	filter := domain.SpaceFilter{
		ProjectID: strings.TrimSpace(p.ProjectID),
		Status:    strings.TrimSpace(p.Status),
		Limit:     p.Limit,
		Offset:    p.Offset,
	}

	spaces, err := h.svc.List(ctx, filter)
	if err != nil {
		return SpaceListResult{}, err
	}
	views := make([]SpaceView, 0, len(spaces))
	for _, space := range spaces {
		views = append(views, NewSpaceView(space))
	}
	return SpaceListResult{Spaces: views, TotalCount: len(views)}, nil
}

func (h *Handler) SpaceUpdate(ctx context.Context, p SpaceUpdateParams) (SpaceUpdateResult, error) {
	id, err := requireSpaceID(p.SpaceID)
	if err != nil {
		return SpaceUpdateResult{}, err
	}

	params := spaceapp.UpdateParams{}
	if p.Title != "" {
		title := strings.TrimSpace(p.Title)
		params.Title = &title
	}
	if p.PlanMode != "" {
		planMode := strings.TrimSpace(p.PlanMode)
		params.PlanMode = &planMode
	}
	if p.Customization != nil {
		params.Customization = p.Customization
	}

	space, err := h.svc.Update(ctx, id, params)
	if err != nil {
		return SpaceUpdateResult{}, err
	}
	return SpaceUpdateResult{Space: NewSpaceView(space)}, nil
}

func (h *Handler) SpaceClose(ctx context.Context, p SpaceCloseParams) (SpaceCloseResult, error) {
	id, err := requireSpaceID(p.SpaceID)
	if err != nil {
		return SpaceCloseResult{}, err
	}

	space, err := h.svc.Close(ctx, id)
	if err != nil {
		return SpaceCloseResult{}, err
	}
	return SpaceCloseResult{Space: NewSpaceView(space)}, nil
}

func (h *Handler) SpaceReopen(ctx context.Context, p SpaceReopenParams) (SpaceReopenResult, error) {
	id, err := requireSpaceID(p.SpaceID)
	if err != nil {
		return SpaceReopenResult{}, err
	}

	space, err := h.svc.Reopen(ctx, id)
	if err != nil {
		return SpaceReopenResult{}, err
	}
	return SpaceReopenResult{Space: NewSpaceView(space)}, nil
}

func (h *Handler) SpaceDelete(ctx context.Context, p SpaceDeleteParams) (SpaceDeleteResult, error) {
	id, err := requireSpaceID(p.SpaceID)
	if err != nil {
		return SpaceDeleteResult{}, err
	}

	if err := h.svc.Delete(ctx, id); err != nil {
		return SpaceDeleteResult{}, err
	}
	return SpaceDeleteResult{SpaceID: string(id)}, nil
}

func (h *Handler) MemberRegister(ctx context.Context, p MemberRegisterParams) (MemberRegisterResult, error) {
	spaceID := strings.TrimSpace(p.SpaceID)
	if spaceID == "" {
		return MemberRegisterResult{}, invalidParams("spaceId is required")
	}
	rosterMember := member.Record{
		SpaceID:        spaceID,
		ProjectID:      strings.TrimSpace(p.ProjectID),
		DisplayName:    strings.TrimSpace(p.DisplayName),
		MemberType:     strings.TrimSpace(p.RequestedMemberType),
		HarnessKind:    strings.TrimSpace(p.HarnessKind),
		Model:          strings.TrimSpace(p.Model),
		Effort:         strings.TrimSpace(p.Effort),
		PermissionMode: strings.TrimSpace(p.PermissionMode),
		ConfigRef:      strings.TrimSpace(p.ConfigRef),
	}

	result, err := h.svc.RegisterMember(ctx, rosterMember)
	if err != nil {
		return MemberRegisterResult{}, err
	}
	return MemberRegisterResult{
		Member:            NewMemberView(result.Member),
		GrantedMemberType: result.GrantedMemberType,
	}, nil
}

func (h *Handler) MemberGet(ctx context.Context, p MemberGetParams) (MemberGetResult, error) {
	id, err := requireMemberID(p.MemberID)
	if err != nil {
		return MemberGetResult{}, err
	}
	rosterMember, err := h.svc.GetMember(ctx, id)
	if err != nil {
		return MemberGetResult{}, err
	}
	return MemberGetResult{Member: NewMemberView(rosterMember)}, nil
}

func (h *Handler) MemberList(ctx context.Context, p MemberListParams) (MemberListResult, error) {
	filter := member.Filter{
		SpaceID:        strings.TrimSpace(p.SpaceID),
		ProjectID:      strings.TrimSpace(p.ProjectID),
		UserID:         strings.TrimSpace(p.UserID),
		MemberType:     strings.TrimSpace(p.MemberType),
		LifecycleState: strings.TrimSpace(p.LifecycleState),
		Limit:          p.Limit,
		Offset:         p.Offset,
	}
	members, err := h.svc.ListMembers(ctx, filter)
	if err != nil {
		return MemberListResult{}, err
	}
	views := make([]MemberView, 0, len(members))
	for _, rosterMember := range members {
		views = append(views, NewMemberView(rosterMember))
	}
	return MemberListResult{Members: views}, nil
}

func (h *Handler) MemberUpdateConfig(ctx context.Context, p MemberUpdateConfigParams) (MemberUpdateConfigResult, error) {
	id, err := requireMemberID(p.MemberID)
	if err != nil {
		return MemberUpdateConfigResult{}, err
	}
	rosterMember, err := h.svc.UpdateMemberConfig(ctx, id, p.Model, p.Effort, p.HarnessKind, p.PermissionMode, p.ConfigRef)
	if err != nil {
		return MemberUpdateConfigResult{}, err
	}
	return MemberUpdateConfigResult{Member: NewMemberView(rosterMember)}, nil
}

func (h *Handler) MemberRemove(ctx context.Context, p MemberRemoveParams) (MemberRemoveResult, error) {
	id, err := requireMemberID(p.MemberID)
	if err != nil {
		return MemberRemoveResult{}, err
	}
	rosterMember, err := h.svc.RemoveMember(ctx, id)
	if err != nil {
		return MemberRemoveResult{}, err
	}
	return MemberRemoveResult{Member: NewMemberView(rosterMember)}, nil
}

func requireSpaceID(raw string) (domain.SpaceID, error) {
	id := domain.SpaceID(strings.TrimSpace(raw))
	if id == "" {
		return "", invalidParams("spaceId is required")
	}
	return id, nil
}

func requireMemberID(raw string) (member.ID, error) {
	id := member.ID(strings.TrimSpace(raw))
	if id == "" {
		return "", invalidParams("memberId is required")
	}
	return id, nil
}

func invalidParams(message string) error {
	return rpcError{code: -32602, message: strings.TrimSpace(message)}
}

type rpcError struct {
	code    int
	message string
}

func (e rpcError) Error() string {
	return e.message
}

func (e rpcError) RPCCode() int {
	return e.code
}
