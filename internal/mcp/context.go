package mcp

import "context"

type sessionContextKey struct{}

func sessionFromContext(ctx context.Context) (Session, bool) {
	if ctx == nil {
		return Session{}, false
	}
	session, ok := ctx.Value(sessionContextKey{}).(Session)
	if !ok {
		return Session{}, false
	}
	return session, true
}
