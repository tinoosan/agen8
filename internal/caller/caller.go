package caller

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

type contextKey struct{}

type Caller struct {
	UserID    string
	MemberID  member.ID
	ProjectID types.ProjectID
	Role      string
}

type Resolver interface {
	ResolveCaller(ctx context.Context) (Caller, error)
}

func ContextWithCaller(ctx context.Context, caller Caller) context.Context {
	return context.WithValue(ctx, contextKey{}, caller)
}

type ContextResolver struct{}

func (ContextResolver) ResolveCaller(ctx context.Context) (Caller, error) {
	caller, ok := ctx.Value(contextKey{}).(Caller)
	if !ok {
		return Caller{}, fmt.Errorf("caller is required")
	}
	caller = caller.Normalize()
	if caller.UserID == "" && caller.MemberID == "" {
		return Caller{}, fmt.Errorf("caller user id or member id is required")
	}
	return caller, nil
}

func (c Caller) Normalize() Caller {
	c.UserID = strings.TrimSpace(c.UserID)
	c.MemberID = member.ID(strings.TrimSpace(string(c.MemberID)))
	c.ProjectID = types.ProjectID(strings.TrimSpace(string(c.ProjectID)))
	c.Role = strings.TrimSpace(c.Role)
	return c
}

func (c Caller) ActorID() string {
	c = c.Normalize()
	if c.MemberID != "" {
		return string(c.MemberID)
	}
	return c.UserID
}
