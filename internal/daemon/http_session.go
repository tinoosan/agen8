package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/rpc"
)

func (d *Daemon) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt.UTC(),
		HttpOnly: true,
		Secure:   d.secureSessionCookie(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func (d *Daemon) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0).UTC(),
		HttpOnly: true,
		Secure:   d.secureSessionCookie(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func (d *Daemon) secureSessionCookie(r *http.Request) bool {
	return requestUsesHTTPS(r) || strings.HasPrefix(strings.ToLower(strings.TrimSpace(d.cfg.PublicURL)), "https://")
}

func sessionTokenFromRequest(r *http.Request) (string, error) {
	if r == nil {
		return "", fmt.Errorf("session cookie is required")
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", fmt.Errorf("session cookie is required")
	}
	return strings.TrimSpace(cookie.Value), nil
}

func requestHasSessionCookie(r *http.Request) bool {
	_, err := sessionTokenFromRequest(r)
	return err == nil
}

func sanitizeLoginResponse(raw []byte) (response []byte, token string, expiresAt time.Time, succeeded bool) {
	var envelope rpc.Response
	if json.Unmarshal(raw, &envelope) != nil || envelope.Error != nil || len(envelope.Result) == 0 {
		return raw, "", time.Time{}, false
	}
	var result map[string]any
	if json.Unmarshal(envelope.Result, &result) != nil {
		return raw, "", time.Time{}, false
	}
	token, _ = result["token"].(string)
	expiresRaw, _ := result["expiresAt"].(string)
	expiresAt, _ = time.Parse(time.RFC3339Nano, expiresRaw)
	if strings.TrimSpace(token) == "" || expiresAt.IsZero() {
		return raw, "", time.Time{}, false
	}
	delete(result, "token")
	sanitizedResult, err := json.Marshal(result)
	if err != nil {
		return raw, "", time.Time{}, false
	}
	envelope.Result = sanitizedResult
	response, err = json.Marshal(envelope)
	if err != nil {
		return raw, "", time.Time{}, false
	}
	return response, token, expiresAt, true
}

func (d *Daemon) handleCookieLogoutRPC(w http.ResponseWriter, r *http.Request, body []byte) {
	var request rpc.Request
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "invalid rpc request", http.StatusBadRequest)
		return
	}
	token, err := sessionTokenFromRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if d == nil || d.app == nil || d.app.AuthSvc == nil || d.app.AuthSvc.RevokeSession(r.Context(), token) != nil {
		d.clearSessionCookie(w, r)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	d.clearSessionCookie(w, r)
	result, _ := json.Marshal(map[string]bool{"revoked": true})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rpc.Response{JSONRPC: "2.0", ID: request.ID, Result: result})
}
