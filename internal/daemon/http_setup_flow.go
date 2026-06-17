package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/services/auth/apikey"
	authapp "github.com/tinoosan/agen8/internal/services/auth/app"
	userapp "github.com/tinoosan/agen8/internal/services/user/app"
	userdomain "github.com/tinoosan/agen8/internal/services/user/domain"
)

type setupRequest struct {
	Token           string `json:"token"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
	KeyName         string `json:"keyName"`
}

type setupCreator struct {
	auth  *authapp.Service
	users *userapp.Service
}

type setupCreationResult struct {
	user             userdomain.User
	sessionToken     string
	sessionExpiresAt time.Time
	apiKey           apikey.Key
	apiKeySecret     string
}

type setupCreateError struct {
	status        int
	clientMessage string
}

func (e setupCreateError) Error() string {
	return e.clientMessage
}

func (h httpSetupHandler) setupCreator() setupCreator {
	return setupCreator{auth: h.auth, users: h.users}
}

func (c setupCreator) create(ctx context.Context, req setupRequest) (setupCreationResult, *setupCreateError) {
	if req.Password != req.ConfirmPassword {
		return setupCreationResult{}, setupCreateClientError(http.StatusBadRequest, "password confirmation does not match")
	}
	if err := c.auth.ValidatePassword(req.Password); err != nil {
		return setupCreationResult{}, setupCreateClientError(http.StatusBadRequest, "invalid password")
	}
	created, err := c.users.SetupFirstUser(ctx, userapp.SetupFirstUserParams{Email: req.Email, Name: req.Name})
	if err != nil {
		return setupCreationResult{}, setupCreateClientError(http.StatusBadRequest, "create setup user")
	}
	if err := c.auth.CreatePassword(ctx, authapp.CreatePasswordParams{UserID: created.User.ID, Password: req.Password}); err != nil {
		return setupCreationResult{}, setupCreateClientError(http.StatusInternalServerError, "create setup credential")
	}
	sessionResult, err := c.auth.CreateSession(ctx, authapp.CreateSessionParams{UserID: created.User.ID})
	if err != nil {
		return setupCreationResult{}, setupCreateClientError(http.StatusInternalServerError, "create setup session")
	}
	keyName := strings.TrimSpace(req.KeyName)
	if keyName == "" {
		keyName = "initial daemon key"
	}
	apiKeyResult, err := c.auth.CreateAPIKey(ctx, authapp.CreateAPIKeyParams{UserID: created.User.ID, Name: keyName})
	if err != nil {
		return setupCreationResult{}, setupCreateClientError(http.StatusInternalServerError, "create setup api key")
	}
	return setupCreationResult{
		user:             created.User,
		sessionToken:     sessionResult.Token,
		sessionExpiresAt: sessionResult.Session.ExpiresAt,
		apiKey:           apiKeyResult.APIKey,
		apiKeySecret:     apiKeyResult.Token,
	}, nil
}

func setupCreateClientError(status int, message string) *setupCreateError {
	return &setupCreateError{status: status, clientMessage: message}
}

func setupWantsJSON(r *http.Request) bool {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(contentType, "application/json") || strings.Contains(accept, "application/json")
}

func decodeSetupRequest(r *http.Request) (setupRequest, error) {
	var req setupRequest
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return req, fmt.Errorf("invalid setup json")
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return req, fmt.Errorf("invalid setup form")
	}
	req.Token = r.Form.Get("token")
	req.Email = r.Form.Get("email")
	req.Name = r.Form.Get("name")
	req.Password = r.Form.Get("password")
	req.ConfirmPassword = r.Form.Get("confirmPassword")
	req.KeyName = r.Form.Get("keyName")
	return req, nil
}
