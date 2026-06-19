package domain

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
)

type TaskFilter struct {
	ProjectID      types.ProjectID
	AssignedTo     member.ID
	ClaimedBy      member.ID
	TaskKind       string
	Status         []TaskStatus
	FromDate       *time.Time
	ToDate         *time.Time
	Limit          int
	Offset         int
	SortBy         string
	SortDesc       bool
	MetadataFilter map[string]string
}

func MemberIDFromString(id string) member.ID {
	return member.ID(strings.TrimSpace(id))
}

type TaskBlocker struct {
	Kind      string
	ID        string
	Reason    string
	CreatedAt string
}

type TaskBlockerMatch struct {
	Kind string
	ID   string
}

const (
	TaskKindTask      = "task"
	TaskKindHeartbeat = "heartbeat"
	TaskKindScheduled = "scheduled"

	TaskMetadataBlockedBy        = "blockedBy"
	TaskMetadataBlockedSignature = "blockedSignature"
)

var (
	ErrTaskNotFound        = errors.New("task not found")
	ErrTaskClaimed         = errors.New("task already claimed by another member")
	ErrTaskBlocked         = errors.New("task is blocked")
	ErrTaskTerminal        = errors.New("task is terminal")
	ErrInvalidFilter       = errors.New("invalid task filter")
	ErrTaskMissingMessage  = errors.New("task projection has no backing message")
	ErrTaskIntentDuplicate = errors.New("task with same intent already exists")
)

type PreclaimedMessage struct {
	MessageID  string
	LeaseOwner string
}

type preclaimedMessageKey struct{}

func WithPreclaimedMessage(ctx context.Context, msg PreclaimedMessage) context.Context {
	msg.MessageID = strings.TrimSpace(msg.MessageID)
	msg.LeaseOwner = strings.TrimSpace(msg.LeaseOwner)
	if msg.MessageID == "" {
		return ctx
	}
	return context.WithValue(ctx, preclaimedMessageKey{}, msg)
}

func PreclaimedMessageFromContext(ctx context.Context) (PreclaimedMessage, bool) {
	msg, ok := ctx.Value(preclaimedMessageKey{}).(PreclaimedMessage)
	if !ok {
		return PreclaimedMessage{}, false
	}
	msg.MessageID = strings.TrimSpace(msg.MessageID)
	msg.LeaseOwner = strings.TrimSpace(msg.LeaseOwner)
	if msg.MessageID == "" {
		return PreclaimedMessage{}, false
	}
	return msg, true
}
