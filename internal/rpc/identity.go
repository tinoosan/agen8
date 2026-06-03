package rpc

import (
	"context"
	"strings"
)

type identityContextKey struct{}

type Identity struct {
	UserID   string
	MemberID string
	Role     string
}

func ContextWithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, identity.normalize())
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(Identity)
	if !ok {
		return Identity{}, false
	}
	return identity.normalize(), true
}

func RequireIdentity(ctx context.Context) (Identity, error) {
	identity, ok := IdentityFromContext(ctx)
	if !ok || identity.IsZero() {
		return Identity{}, InvalidRequest("rpc identity is required")
	}
	return identity, nil
}

func (i Identity) IsZero() bool {
	i = i.normalize()
	return i.UserID == "" && i.MemberID == ""
}

func (i Identity) normalize() Identity {
	i.UserID = strings.TrimSpace(i.UserID)
	i.MemberID = strings.TrimSpace(i.MemberID)
	i.Role = strings.TrimSpace(i.Role)
	return i
}
