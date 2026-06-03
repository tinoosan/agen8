package rpc

import (
	"context"
	"fmt"
	"strings"

	locationapp "github.com/tinoosan/agen8-mcp-server/internal/services/location/app"
	locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"
)

type Handler struct {
	service *locationapp.Service
}

func NewHandler(service *locationapp.Service) (*Handler, error) {
	if service == nil {
		return nil, fmt.Errorf("location service is required")
	}
	return &Handler{service: service}, nil
}

func MustNewHandler(service *locationapp.Service) *Handler {
	handler, err := NewHandler(service)
	if err != nil {
		panic(err)
	}
	return handler
}

func (h *Handler) LocationList(ctx context.Context, p LocationListParams) (LocationListResult, error) {
	locations, err := h.service.ListLocations(ctx, locationdomain.Filter{
		Kind:   locationdomain.Kind(strings.TrimSpace(p.Kind)),
		Status: locationdomain.Status(strings.TrimSpace(p.Status)),
		Ready:  p.Ready,
		Limit:  p.Limit,
		Offset: p.Offset,
	})
	if err != nil {
		return LocationListResult{}, internalError("list locations", err)
	}
	views := make([]LocationView, 0, len(locations))
	for _, location := range locations {
		views = append(views, NewLocationView(location))
	}
	return LocationListResult{Locations: views}, nil
}

func (h *Handler) LocationGet(ctx context.Context, p LocationGetParams) (LocationResult, error) {
	id, err := requireLocationID(p.LocationID)
	if err != nil {
		return LocationResult{}, err
	}
	location, err := h.service.GetLocation(ctx, id)
	if err != nil {
		return LocationResult{}, internalError("get location", err)
	}
	return LocationResult{Location: NewLocationView(location)}, nil
}

func (h *Handler) LocationCreate(ctx context.Context, p LocationCreateParams) (LocationResult, error) {
	kind := strings.TrimSpace(p.Kind)
	if kind == "" {
		return LocationResult{}, invalidParams("kind is required")
	}
	location, err := h.service.CreateLocation(ctx, locationapp.CreateLocationInput{
		Kind:          locationdomain.Kind(kind),
		Label:         strings.TrimSpace(p.Label),
		Address:       locationAddress(p.Address),
		CredentialRef: strings.TrimSpace(p.Auth.CredentialID),
	})
	if err != nil {
		return LocationResult{}, internalError("create location", err)
	}
	return LocationResult{Location: NewLocationView(location)}, nil
}

func (h *Handler) LocationUpdate(ctx context.Context, p LocationUpdateParams) (LocationResult, error) {
	id, err := requireLocationID(p.LocationID)
	if err != nil {
		return LocationResult{}, err
	}
	var address *locationdomain.Address
	if p.Address != nil {
		converted := locationAddress(*p.Address)
		address = &converted
	}
	location, err := h.service.UpdateLocation(ctx, locationapp.UpdateLocationInput{
		ID:            id,
		Label:         strings.TrimSpace(p.Label),
		Address:       address,
		CredentialRef: credentialRefPtr(p.Auth),
	})
	if err != nil {
		return LocationResult{}, internalError("update location", err)
	}
	return LocationResult{Location: NewLocationView(location)}, nil
}

func credentialRefPtr(auth *LocationAuthView) *string {
	if auth == nil {
		return nil
	}
	value := strings.TrimSpace(auth.CredentialID)
	return &value
}

func (h *Handler) LocationDelete(ctx context.Context, p LocationDeleteParams) (struct{}, error) {
	id, err := requireLocationID(p.LocationID)
	if err != nil {
		return struct{}{}, err
	}
	if err := h.service.DeleteLocation(ctx, id); err != nil {
		return struct{}{}, internalError("delete location", err)
	}
	return struct{}{}, nil
}

func (h *Handler) LocationProbe(ctx context.Context, p LocationProbeParams) (LocationResult, error) {
	id, err := requireLocationID(p.LocationID)
	if err != nil {
		return LocationResult{}, err
	}
	location, err := h.service.ProbeLocation(ctx, id)
	if err != nil {
		return LocationResult{}, internalError("probe location", err)
	}
	return LocationResult{Location: NewLocationView(location)}, nil
}

func (h *Handler) LocationInstallCodex(ctx context.Context, p LocationInstallCodexParams) (LocationResult, error) {
	id, err := requireLocationID(p.LocationID)
	if err != nil {
		return LocationResult{}, err
	}
	location, err := h.service.InstallCodex(ctx, id)
	if err != nil {
		return LocationResult{}, rpcError{code: -32000, message: fmt.Sprintf("install codex: %v", err)}
	}
	return LocationResult{Location: NewLocationView(location)}, nil
}

func (h *Handler) LocationCodexAuthStatus(ctx context.Context, p LocationCodexAuthStatusParams) (LocationCodexAuthStatusResult, error) {
	id, err := requireLocationID(p.LocationID)
	if err != nil {
		return LocationCodexAuthStatusResult{}, err
	}
	status, err := h.service.CodexAuthStatus(ctx, id)
	if err != nil {
		return LocationCodexAuthStatusResult{}, rpcError{code: -32000, message: fmt.Sprintf("check Codex auth: %v", err)}
	}
	return LocationCodexAuthStatusResult{
		LoggedIn: status.LoggedIn,
		Method:   status.Method,
		Output:   status.Output,
	}, nil
}

func (h *Handler) LocationCodexLogin(ctx context.Context, p LocationCodexLoginParams) (LocationCodexLoginResult, error) {
	id, err := requireLocationID(p.LocationID)
	if err != nil {
		return LocationCodexLoginResult{}, err
	}
	login, err := h.service.BeginCodexLogin(ctx, id)
	if err != nil {
		return LocationCodexLoginResult{}, rpcError{code: -32000, message: fmt.Sprintf("begin Codex login: %v", err)}
	}
	return LocationCodexLoginResult{
		Output:   login.Output,
		LoginURL: login.LoginURL,
		LogPath:  login.LogPath,
		PID:      login.PID,
	}, nil
}

func (h *Handler) LocationInstallClaude(ctx context.Context, p LocationInstallClaudeParams) (LocationResult, error) {
	id, err := requireLocationID(p.LocationID)
	if err != nil {
		return LocationResult{}, err
	}
	location, err := h.service.InstallClaude(ctx, id)
	if err != nil {
		return LocationResult{}, rpcError{code: -32000, message: fmt.Sprintf("install Claude Code: %v", err)}
	}
	return LocationResult{Location: NewLocationView(location)}, nil
}

func (h *Handler) LocationClaudeAuthStatus(ctx context.Context, p LocationClaudeAuthStatusParams) (LocationClaudeAuthStatusResult, error) {
	id, err := requireLocationID(p.LocationID)
	if err != nil {
		return LocationClaudeAuthStatusResult{}, err
	}
	status, err := h.service.ClaudeAuthStatus(ctx, id)
	if err != nil {
		return LocationClaudeAuthStatusResult{}, rpcError{code: -32000, message: fmt.Sprintf("check Claude Code auth: %v", err)}
	}
	return LocationClaudeAuthStatusResult{
		LoggedIn:   status.LoggedIn,
		AuthMethod: status.AuthMethod,
		Provider:   status.Provider,
		RawJSON:    status.RawJSON,
	}, nil
}

func (h *Handler) LocationClaudeLogin(ctx context.Context, p LocationClaudeLoginParams) (LocationClaudeLoginResult, error) {
	id, err := requireLocationID(p.LocationID)
	if err != nil {
		return LocationClaudeLoginResult{}, err
	}
	login, err := h.service.BeginClaudeLogin(ctx, id)
	if err != nil {
		return LocationClaudeLoginResult{}, rpcError{code: -32000, message: fmt.Sprintf("begin Claude Code login: %v", err)}
	}
	return LocationClaudeLoginResult{
		Output:   login.Output,
		LoginURL: login.LoginURL,
		LogPath:  login.LogPath,
		PID:      login.PID,
	}, nil
}

func (h *Handler) LocationClaudeLoginComplete(ctx context.Context, p LocationClaudeLoginCompleteParams) (LocationClaudeLoginResult, error) {
	id, err := requireLocationID(p.LocationID)
	if err != nil {
		return LocationClaudeLoginResult{}, err
	}
	if strings.TrimSpace(p.Code) == "" {
		return LocationClaudeLoginResult{}, invalidParams("code is required")
	}
	login, err := h.service.CompleteClaudeLogin(ctx, id, p.Code)
	if err != nil {
		return LocationClaudeLoginResult{}, rpcError{code: -32000, message: fmt.Sprintf("complete Claude Code login: %v", err)}
	}
	return LocationClaudeLoginResult{
		Output:   login.Output,
		LoginURL: login.LoginURL,
		LogPath:  login.LogPath,
		PID:      login.PID,
	}, nil
}

func (h *Handler) LocationFSListDir(ctx context.Context, p LocationFSListDirParams) (LocationFSListDirResult, error) {
	id, err := requireLocationID(p.LocationID)
	if err != nil {
		return LocationFSListDirResult{}, err
	}
	if strings.TrimSpace(p.Path) == "" {
		return LocationFSListDirResult{}, invalidParams("path is required")
	}
	entries, err := h.service.ListDir(ctx, id, p.Path)
	if err != nil {
		return LocationFSListDirResult{}, internalError("list location directory", err)
	}
	out := make([]DirEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, DirEntry{
			Name: entry.Name,
			Path: entry.Path,
			Type: string(entry.Type),
			Size: entry.Size,
		})
	}
	return LocationFSListDirResult{Entries: out}, nil
}

func requireLocationID(value string) (locationdomain.ID, error) {
	id := locationdomain.ID(strings.TrimSpace(value))
	if id == "" {
		return "", invalidParams("locationId is required")
	}
	return id, nil
}

func locationAddress(view LocationAddressView) locationdomain.Address {
	return locationdomain.Address{
		Host:     strings.TrimSpace(view.Host),
		Port:     view.Port,
		Username: strings.TrimSpace(view.Username),
	}
}

func invalidParams(message string) error {
	return rpcError{code: -32602, message: strings.TrimSpace(message)}
}

func internalError(action string, err error) error {
	return fmt.Errorf("%s: %w", action, err)
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
