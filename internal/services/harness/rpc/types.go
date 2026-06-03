package rpc

import (
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	harnessrun "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain/run"
)

type ConfigOptionsParams struct{}

type ConfigOptionsResult struct {
	Harnesses []HarnessOption `json:"harnesses"`
}

type HarnessOption struct {
	Kind            string                 `json:"kind"`
	Models          []ModelOption          `json:"models"`
	PermissionModes []PermissionModeOption `json:"permissionModes"`
}

type ModelOption struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
	Efforts []string `json:"efforts"`
}

type PermissionModeOption struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	Default           bool   `json:"default"`
	RequiresConfigRef bool   `json:"requiresConfigRef,omitempty"`
}

type SessionGetParams struct {
	SessionID string `json:"sessionId"`
}

type SessionGetResult struct {
	Session SessionView `json:"session"`
}

type SessionListParams struct {
	SpaceID  string `json:"spaceId,omitempty"`
	MemberID string `json:"memberId,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type SessionListResult struct {
	Sessions []SessionView `json:"sessions"`
}

type RunListParams struct {
	ProjectID string   `json:"projectId,omitempty"`
	SpaceID   string   `json:"spaceId,omitempty"`
	ChannelID string   `json:"channelId,omitempty"`
	MemberID  string   `json:"memberId,omitempty"`
	SessionID string   `json:"sessionId,omitempty"`
	Status    []string `json:"status,omitempty"`
	Limit     int      `json:"limit,omitempty"`
}

type RunListResult struct {
	Runs []RunView `json:"runs"`
}

type TurnCancelParams struct {
	RunID     string `json:"runId,omitempty"`
	TurnID    string `json:"turnId,omitempty"`
	ChannelID string `json:"channelId,omitempty"`
}

type TurnCancelResult struct {
	Run RunView `json:"run"`
}

type SessionView struct {
	ID             string     `json:"id"`
	MemberID       string     `json:"memberId"`
	SpaceID        string     `json:"spaceId"`
	Status         string     `json:"status"`
	InactiveReason string     `json:"inactiveReason,omitempty"`
	InactiveError  string     `json:"inactiveError,omitempty"`
	ActivatedAt    time.Time  `json:"activatedAt"`
	DeactivatedAt  *time.Time `json:"deactivatedAt,omitempty"`
	TokensIn       int64      `json:"tokensIn"`
	TokensOut      int64      `json:"tokensOut"`
	Kind           string     `json:"kind"`
	Model          string     `json:"model"`
	Effort         string     `json:"effort"`
	PermissionMode string     `json:"harnessPermissionMode,omitempty"`
	ConfigRef      string     `json:"harnessConfigRef,omitempty"`
	Ref            string     `json:"ref,omitempty"`
}

func NewSessionView(session *domain.Session) SessionView {
	if session == nil {
		return SessionView{}
	}
	return SessionView{
		ID:             session.ID,
		MemberID:       session.MemberID,
		SpaceID:        session.SpaceID,
		Status:         string(session.Status),
		InactiveReason: string(session.InactiveReason),
		InactiveError:  session.InactiveError,
		ActivatedAt:    session.ActivatedAt,
		DeactivatedAt:  session.DeactivatedAt,
		TokensIn:       session.TokensIn,
		TokensOut:      session.TokensOut,
		Kind:           session.Kind,
		Model:          session.Model,
		Effort:         session.Effort,
		PermissionMode: session.PermissionMode,
		ConfigRef:      session.ConfigRef,
		Ref:            session.Ref,
	}
}

type RunView struct {
	ID               string     `json:"id"`
	ProjectID        string     `json:"projectId"`
	SpaceID          string     `json:"spaceId"`
	ChannelID        string     `json:"channelId"`
	MemberID         string     `json:"memberId"`
	SessionID        string     `json:"sessionId"`
	HarnessKind      string     `json:"harnessKind"`
	NativeSessionRef string     `json:"nativeSessionRef,omitempty"`
	TurnID           string     `json:"turnId"`
	NativeTurnID     string     `json:"nativeTurnId,omitempty"`
	Status           string     `json:"status"`
	StopRequestedBy  string     `json:"stopRequestedBy,omitempty"`
	StopRequestedAt  *time.Time `json:"stopRequestedAt,omitempty"`
	StartedAt        time.Time  `json:"startedAt"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
	Error            string     `json:"error,omitempty"`
}

func NewRunView(run harnessrun.Run) RunView {
	return RunView{
		ID:               run.ID,
		ProjectID:        run.ProjectID,
		SpaceID:          run.SpaceID,
		ChannelID:        run.ChannelID,
		MemberID:         run.MemberID,
		SessionID:        run.SessionID,
		HarnessKind:      run.HarnessKind,
		NativeSessionRef: run.NativeSessionRef,
		TurnID:           run.TurnID,
		NativeTurnID:     run.NativeTurnID,
		Status:           string(run.Status),
		StopRequestedBy:  run.StopRequestedBy,
		StopRequestedAt:  run.StopRequestedAt,
		StartedAt:        run.StartedAt,
		CompletedAt:      run.CompletedAt,
		Error:            run.Error,
	}
}

func runViews(rows []harnessrun.Run) []RunView {
	views := make([]RunView, 0, len(rows))
	for _, row := range rows {
		views = append(views, NewRunView(row))
	}
	return views
}
