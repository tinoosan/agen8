package daemon

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/tinoosan/agen8/internal/rpc"
	authapp "github.com/tinoosan/agen8/internal/services/auth/app"
	userdomain "github.com/tinoosan/agen8/internal/services/user/domain"
)

const sessionCookieName = "agen8.sessionToken"

type httpIdentityResolver struct {
	auth *authapp.Service
}

func (d *Daemon) httpIdentityResolver() httpIdentityResolver {
	if d == nil || d.app == nil {
		return httpIdentityResolver{}
	}
	return httpIdentityResolver{auth: d.app.AuthSvc}
}

func (d *Daemon) httpIdentity(ctx context.Context, authorization string) (rpc.Identity, error) {
	return d.httpIdentityResolver().ResolveBearerSession(ctx, authorization)
}

func (d *Daemon) httpIdentityFromSessionCookie(ctx context.Context, r *http.Request) (rpc.Identity, error) {
	return d.httpIdentityResolver().ResolveSessionCookie(ctx, r)
}

func (d *Daemon) attentionHookUserID(r *http.Request) string {
	if r == nil {
		return ""
	}
	return d.httpIdentityResolver().ResolveAttentionHookUserID(r.Context(), r.Header.Get("Authorization"))
}

func (r httpIdentityResolver) ResolveBearerSession(ctx context.Context, authorization string) (rpc.Identity, error) {
	token := bearerToken(authorization)
	if token == "" {
		return rpc.Identity{}, fmt.Errorf("bearer token is required")
	}
	return r.resolveSessionToken(ctx, token)
}

func (r httpIdentityResolver) ResolveSessionCookie(ctx context.Context, req *http.Request) (rpc.Identity, error) {
	if req == nil {
		return rpc.Identity{}, fmt.Errorf("session cookie is required")
	}
	cookie, err := req.Cookie(sessionCookieName)
	if err != nil {
		return rpc.Identity{}, fmt.Errorf("session cookie is required")
	}
	token := strings.TrimSpace(cookie.Value)
	if token == "" {
		return rpc.Identity{}, fmt.Errorf("session cookie is empty")
	}
	return r.resolveSessionToken(ctx, token)
}

// ResolveAttentionHookUserID accepts the same bearer header shape as /rpc and
// /events, but preserves the hook-specific token order: MCP API key first, then
// web session token.
func (r httpIdentityResolver) ResolveAttentionHookUserID(ctx context.Context, authorization string) string {
	token := bearerToken(authorization)
	if token == "" || r.auth == nil {
		return ""
	}
	if account, err := r.auth.ValidateAPIKey(ctx, token); err == nil {
		return strings.TrimSpace(account.ID.String())
	}
	if identity, err := r.resolveSessionToken(ctx, token); err == nil {
		return strings.TrimSpace(identity.UserID)
	}
	return ""
}

func (r httpIdentityResolver) resolveSessionToken(ctx context.Context, token string) (rpc.Identity, error) {
	if r.auth == nil {
		return rpc.Identity{}, fmt.Errorf("auth service is required")
	}
	user, err := r.auth.ValidateSession(ctx, strings.TrimSpace(token))
	if err != nil {
		return rpc.Identity{}, err
	}
	role := string(userdomain.RoleUser)
	if user.Role != "" {
		role = string(user.Role)
	}
	return rpc.Identity{UserID: user.ID.String(), Role: role}, nil
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}
