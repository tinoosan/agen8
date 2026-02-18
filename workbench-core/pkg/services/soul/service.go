package soul

import (
	"context"
	"time"
)

type ActorLayer string

const (
	ActorOperator ActorLayer = "operator"
	ActorAgent    ActorLayer = "agent"
	ActorPolicy   ActorLayer = "policy"
	ActorDaemon   ActorLayer = "daemon"
)

type Doc struct {
	Content   string
	Version   int
	Checksum  string
	UpdatedAt time.Time
	UpdatedBy ActorLayer
	Locked    bool
}

type UpdateRequest struct {
	Content         string
	Reason          string
	Actor           ActorLayer
	ExpectedVersion int
	OverrideLock    bool
	AllowImmutable  bool
}

type AuditEvent struct {
	ID             string     `json:"id"`
	Timestamp      time.Time  `json:"timestamp"`
	ActorLayer     ActorLayer `json:"actorLayer"`
	Action         string     `json:"action"`
	Reason         string     `json:"reason,omitempty"`
	VersionBefore  int        `json:"versionBefore"`
	VersionAfter   int        `json:"versionAfter"`
	ChecksumBefore string     `json:"checksumBefore,omitempty"`
	ChecksumAfter  string     `json:"checksumAfter,omitempty"`
}

type Service interface {
	Get(ctx context.Context) (Doc, error)
	Update(ctx context.Context, req UpdateRequest) (Doc, error)
	History(ctx context.Context, limit int, cursor string) ([]AuditEvent, string, error)
	SetLock(ctx context.Context, locked bool, actor ActorLayer, reason string) (Doc, error)
}
