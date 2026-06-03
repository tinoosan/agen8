package userctx

import (
	"context"
	"strings"
)

const LocalUserID = "local"

type contextKey struct{}

func WithUserID(ctx context.Context, userID string) context.Context {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, userID)
}

func UserID(ctx context.Context) string {
	userID, _ := ctx.Value(contextKey{}).(string)
	return strings.TrimSpace(userID)
}

func EffectiveUserID(ctx context.Context, explicit string) string {
	if userID := strings.TrimSpace(explicit); userID != "" {
		return userID
	}
	if userID := UserID(ctx); userID != "" {
		return userID
	}
	return LocalUserID
}
