package member

import (
	"fmt"
	"strings"
	"time"
)

type ID string

func (id ID) String() string { return string(id) }

const (
	TypeCoordinator = "coordinator"
	TypeWorker      = "worker"

	LifecycleActive  = "active"
	LifecycleRemoved = "removed"
)

type Record struct {
	ID               ID         `json:"id"`
	UserID           string     `json:"userId,omitempty"`
	ProjectID        string     `json:"projectId"`
	NativeSessionRef string     `json:"nativeSessionRef,omitempty"`
	ChannelID        string     `json:"channelId"`
	DisplayName      string     `json:"displayName"`
	MemberType       string     `json:"memberType"`
	LifecycleState   string     `json:"lifecycleState"`
	HarnessKind      string     `json:"harnessKind"`
	Model            string     `json:"model"`
	Effort           string     `json:"effort"`
	PermissionMode   string     `json:"harnessPermissionMode,omitempty"`
	ConfigRef        string     `json:"harnessConfigRef,omitempty"`
	RegisteredAt     time.Time  `json:"registeredAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	LastSeenAt       *time.Time `json:"lastSeenAt,omitempty"`
}

type Member struct {
	inner Record
}

func WrapMember(m Record) Member {
	if strings.TrimSpace(m.LifecycleState) == "" {
		m.LifecycleState = LifecycleActive
	}
	return Member{inner: m}
}

func (a Member) Inner() Record      { return a.inner }
func (a Member) ID() string         { return string(a.inner.ID) }
func (a Member) ProjectID() string  { return a.inner.ProjectID }
func (a Member) MemberType() string { return a.inner.MemberType }
func (a Member) IsActive() bool     { return a.inner.LifecycleState == LifecycleActive }

func (a Member) Remove(now time.Time) (Member, error) {
	if a.inner.LifecycleState == LifecycleRemoved {
		return Member{}, fmt.Errorf("member is already removed")
	}
	next := a.inner
	next.LifecycleState = LifecycleRemoved
	next.UpdatedAt = now.UTC()
	return Member{inner: next}, nil
}

func (a Member) SetMemberType(memberType string, now time.Time) (Member, error) {
	if err := ValidateMemberType(memberType); err != nil {
		return Member{}, err
	}
	next := a.inner
	next.MemberType = memberType
	next.UpdatedAt = now.UTC()
	return Member{inner: next}, nil
}

func ValidateMemberType(v string) error {
	switch v {
	case TypeCoordinator, TypeWorker:
		return nil
	default:
		return fmt.Errorf("invalid memberType %q", v)
	}
}

func ValidateLifecycleState(v string) error {
	switch v {
	case LifecycleActive, LifecycleRemoved:
		return nil
	default:
		return fmt.Errorf("invalid lifecycleState %q", v)
	}
}

func IsCoordinatorType(v string) bool {
	return v == TypeCoordinator
}
