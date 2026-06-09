package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/tinoosan/agen8/internal/services/auth/apikey"
	auth "github.com/tinoosan/agen8/internal/services/auth/domain"
	"github.com/tinoosan/agen8/internal/services/auth/linktoken"
	"github.com/tinoosan/agen8/internal/services/auth/password"
	"github.com/tinoosan/agen8/internal/services/auth/session"
	user "github.com/tinoosan/agen8/internal/services/user/domain"
)

const (
	bcryptCost         = 12
	sessionTokenSize   = 32
	apiKeyTokenSize    = 32
	linkTokenTokenSize = 32
	defaultSessionTTL  = 7 * 24 * time.Hour
)

type UserLoader interface {
	Get(ctx context.Context, id user.ID) (user.User, error)
	GetByEmail(ctx context.Context, email string) (user.User, error)
}

type Service struct {
	passwords  password.Repository
	sessions   session.Repository
	apiKeys    apikey.Repository
	linkTokens linktoken.Repository
	users      UserLoader
	clock      auth.Clock
	logger     *slog.Logger
}

func NewService(
	passwords password.Repository,
	sessions session.Repository,
	apiKeys apikey.Repository,
	linkTokens linktoken.Repository,
	users UserLoader,
	clock auth.Clock,
	logger *slog.Logger,
) (*Service, error) {
	if passwords == nil {
		return nil, fmt.Errorf("password repository is required")
	}
	if sessions == nil {
		return nil, fmt.Errorf("session repository is required")
	}
	if apiKeys == nil {
		return nil, fmt.Errorf("api key repository is required")
	}
	if linkTokens == nil {
		return nil, fmt.Errorf("link token repository is required")
	}
	if users == nil {
		return nil, fmt.Errorf("user loader is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("clock is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	return &Service{passwords: passwords, sessions: sessions, apiKeys: apiKeys, linkTokens: linkTokens, users: users, clock: clock, logger: logger}, nil
}

type CreatePasswordParams struct {
	UserID   user.ID
	Password string
}

func (s *Service) CreatePassword(ctx context.Context, params CreatePasswordParams) error {
	if err := s.ValidatePassword(params.Password); err != nil {
		return err
	}
	hash, err := hashPassword(params.Password)
	if err != nil {
		return err
	}
	record, err := password.New(password.NewInput{
		UserID:       params.UserID,
		PasswordHash: hash,
		Now:          s.clock.Now(),
	})
	if err != nil {
		return err
	}
	if err := s.passwords.Save(ctx, record); err != nil {
		return err
	}
	s.logger.Info("auth password credential saved", "user_id", params.UserID.String())
	return nil
}

func (s *Service) ValidatePassword(raw string) error {
	if len(raw) < 8 {
		return auth.ErrPasswordTooShort
	}
	return nil
}

func (s *Service) VerifyPassword(ctx context.Context, userID user.ID, rawPassword string) error {
	record, err := s.passwords.Get(ctx, userID)
	if err != nil {
		return auth.ErrInvalidCredential
	}
	if err := bcrypt.CompareHashAndPassword([]byte(record.PasswordHash), []byte(rawPassword)); err != nil {
		return auth.ErrInvalidCredential
	}
	return nil
}

type LoginParams struct {
	Email      string
	Password   string
	UserAgent  string
	RemoteAddr string
}

type LoginResult struct {
	User    user.User
	Session session.Session
	Token   string
}

func (s *Service) Login(ctx context.Context, params LoginParams) (LoginResult, error) {
	email := strings.ToLower(strings.TrimSpace(params.Email))
	if email == "" {
		return LoginResult{}, auth.ErrInvalidCredential
	}
	account, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return LoginResult{}, auth.ErrInvalidCredential
	}
	if !account.IsActive() {
		return LoginResult{}, auth.ErrUserInactive
	}
	if err := s.VerifyPassword(ctx, account.ID, params.Password); err != nil {
		return LoginResult{}, err
	}
	sessionResult, err := s.CreateSession(ctx, CreateSessionParams{
		UserID:     account.ID,
		UserAgent:  params.UserAgent,
		RemoteAddr: params.RemoteAddr,
	})
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		User:    account,
		Session: sessionResult.Session,
		Token:   sessionResult.Token,
	}, nil
}

type CreateSessionParams struct {
	UserID     user.ID
	UserAgent  string
	RemoteAddr string
	TTL        time.Duration
}

type CreateSessionResult struct {
	Session session.Session
	Token   string
}

func (s *Service) CreateSession(ctx context.Context, params CreateSessionParams) (CreateSessionResult, error) {
	account, err := s.activeUser(ctx, params.UserID)
	if err != nil {
		return CreateSessionResult{}, err
	}
	token, err := auth.NewRawToken("ses", sessionTokenSize)
	if err != nil {
		return CreateSessionResult{}, err
	}
	sessionID, err := session.NewID("session_" + uuid.NewString())
	if err != nil {
		return CreateSessionResult{}, err
	}
	ttl := params.TTL
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	now := s.clock.Now()
	record := session.Session{
		ID:        sessionID,
		UserID:    account.ID,
		TokenHash: auth.HashToken(token),
		ExpiresAt: now.Add(ttl).UTC(),
		CreatedAt: now,
	}
	if err := s.sessions.Create(ctx, record); err != nil {
		return CreateSessionResult{}, err
	}
	s.logger.Info("auth session created", "user_id", account.ID.String())
	return CreateSessionResult{Session: record, Token: token}, nil
}

func (s *Service) ValidateSession(ctx context.Context, token string) (user.User, error) {
	record, err := s.sessions.GetByTokenHash(ctx, auth.HashToken(token))
	if err != nil {
		return user.User{}, auth.ErrTokenNotFound
	}
	if !record.IsActive(s.clock.Now()) {
		return user.User{}, auth.ErrTokenExpired
	}
	return s.activeUser(ctx, record.UserID)
}

func (s *Service) RevokeSession(ctx context.Context, token string) error {
	record, err := s.sessions.GetByTokenHash(ctx, auth.HashToken(token))
	if err != nil {
		return auth.ErrTokenNotFound
	}
	now := s.clock.Now()
	record.RevokedAt = &now
	return s.sessions.Update(ctx, record)
}

type CreateAPIKeyParams struct {
	UserID    user.ID
	Name      string
	ExpiresAt *time.Time
}

type CreateAPIKeyResult struct {
	APIKey apikey.Key
	Token  string
}

func (s *Service) CreateAPIKey(ctx context.Context, params CreateAPIKeyParams) (CreateAPIKeyResult, error) {
	account, err := s.activeUser(ctx, params.UserID)
	if err != nil {
		return CreateAPIKeyResult{}, err
	}
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return CreateAPIKeyResult{}, fmt.Errorf("api key name is required")
	}
	token, err := auth.NewRawToken("ak", apiKeyTokenSize)
	if err != nil {
		return CreateAPIKeyResult{}, err
	}
	keyID, err := apikey.NewID("api_key_" + uuid.NewString())
	if err != nil {
		return CreateAPIKeyResult{}, err
	}
	prefix := token
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	record := apikey.Key{
		ID:        keyID,
		UserID:    account.ID,
		Name:      name,
		Prefix:    prefix,
		TokenHash: auth.HashToken(token),
		ExpiresAt: params.ExpiresAt,
		CreatedAt: s.clock.Now(),
	}
	if err := s.apiKeys.Create(ctx, record); err != nil {
		return CreateAPIKeyResult{}, err
	}
	s.logger.Info("auth api key created", "user_id", account.ID.String())
	return CreateAPIKeyResult{APIKey: record, Token: token}, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, userID user.ID) ([]apikey.Key, error) {
	account, err := s.activeUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.apiKeys.ListByUser(ctx, account.ID)
}

func (s *Service) ValidateAPIKey(ctx context.Context, token string) (user.User, error) {
	record, err := s.apiKeys.GetByTokenHash(ctx, auth.HashToken(token))
	if err != nil {
		return user.User{}, auth.ErrTokenNotFound
	}
	if !record.IsActive(s.clock.Now()) {
		return user.User{}, auth.ErrTokenExpired
	}
	return s.activeUser(ctx, record.UserID)
}

func (s *Service) RevokeAPIKey(ctx context.Context, id apikey.ID) error {
	record, err := s.apiKeys.Get(ctx, id)
	if err != nil {
		return auth.ErrTokenNotFound
	}
	now := s.clock.Now()
	record.RevokedAt = &now
	return s.apiKeys.Update(ctx, record)
}

func (s *Service) RevokeUserAPIKey(ctx context.Context, userID user.ID, id apikey.ID) error {
	account, err := s.activeUser(ctx, userID)
	if err != nil {
		return err
	}
	record, err := s.apiKeys.Get(ctx, id)
	if err != nil {
		return auth.ErrTokenNotFound
	}
	if record.UserID != account.ID {
		return auth.ErrTokenNotFound
	}
	now := s.clock.Now()
	record.RevokedAt = &now
	return s.apiKeys.Update(ctx, record)
}

type CreateLinkTokenParams struct {
	UserID      user.ID
	ProjectID   string
	WorkspaceID string
	Label       string
	ExpiresAt   *time.Time
}

type CreateLinkTokenResult struct {
	LinkToken linktoken.LinkToken
	Token     string
}

// LinkTokenBinding is what a valid wlt_ token resolves to: the user that holds
// it and the project (and optional workspace) it is bound to. ProjectID and
// WorkspaceID are opaque strings owned by the project service.
type LinkTokenBinding struct {
	User        user.User
	ProjectID   string
	WorkspaceID string
}

func (s *Service) CreateLinkToken(ctx context.Context, params CreateLinkTokenParams) (CreateLinkTokenResult, error) {
	account, err := s.activeUser(ctx, params.UserID)
	if err != nil {
		return CreateLinkTokenResult{}, err
	}
	projectID := strings.TrimSpace(params.ProjectID)
	if projectID == "" {
		return CreateLinkTokenResult{}, fmt.Errorf("link token project id is required")
	}
	token, err := auth.NewRawToken("wlt", linkTokenTokenSize)
	if err != nil {
		return CreateLinkTokenResult{}, err
	}
	tokenID, err := linktoken.NewID("link_token_" + uuid.NewString())
	if err != nil {
		return CreateLinkTokenResult{}, err
	}
	prefix := token
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	record := linktoken.LinkToken{
		ID:          tokenID,
		UserID:      account.ID,
		ProjectID:   projectID,
		WorkspaceID: strings.TrimSpace(params.WorkspaceID),
		Label:       strings.TrimSpace(params.Label),
		Prefix:      prefix,
		TokenHash:   auth.HashToken(token),
		ExpiresAt:   params.ExpiresAt,
		CreatedAt:   s.clock.Now(),
	}
	if err := s.linkTokens.Create(ctx, record); err != nil {
		return CreateLinkTokenResult{}, err
	}
	s.logger.Info("auth link token created", "user_id", account.ID.String(), "project_id", projectID)
	return CreateLinkTokenResult{LinkToken: record, Token: token}, nil
}

func (s *Service) ValidateLinkToken(ctx context.Context, token string) (LinkTokenBinding, error) {
	record, err := s.linkTokens.GetByTokenHash(ctx, auth.HashToken(token))
	if err != nil {
		return LinkTokenBinding{}, auth.ErrTokenNotFound
	}
	if !record.IsActive(s.clock.Now()) {
		return LinkTokenBinding{}, auth.ErrTokenExpired
	}
	account, err := s.activeUser(ctx, record.UserID)
	if err != nil {
		return LinkTokenBinding{}, err
	}
	return LinkTokenBinding{
		User:        account,
		ProjectID:   strings.TrimSpace(record.ProjectID),
		WorkspaceID: strings.TrimSpace(record.WorkspaceID),
	}, nil
}

// ListLinkTokensParams narrows which link tokens to return. ProjectID is the
// field the project service sets once it has confirmed the caller owns that
// project; the auth service does not re-check ownership, it just lists.
type ListLinkTokensParams struct {
	ProjectID string
	UserID    user.ID
	Limit     int
	Offset    int
}

// ListLinkTokens returns every link token matching the filter, including
// revoked and expired ones. The caller (the project service) decides how to
// present state; auth's job is only to surface the rows. The raw secret is
// never recoverable here — only the hash is stored — so summaries are safe to
// hand back.
func (s *Service) ListLinkTokens(ctx context.Context, params ListLinkTokensParams) ([]linktoken.LinkToken, error) {
	return s.linkTokens.List(ctx, linktoken.Filter{
		ProjectID: strings.TrimSpace(params.ProjectID),
		UserID:    strings.TrimSpace(params.UserID.String()),
		Limit:     params.Limit,
		Offset:    params.Offset,
	})
}

func (s *Service) RevokeLinkToken(ctx context.Context, id linktoken.ID) error {
	record, err := s.linkTokens.Get(ctx, id)
	if err != nil {
		return auth.ErrTokenNotFound
	}
	now := s.clock.Now()
	record.RevokedAt = &now
	return s.linkTokens.Update(ctx, record)
}

func (s *Service) activeUser(ctx context.Context, id user.ID) (user.User, error) {
	account, err := s.users.Get(ctx, id)
	if err != nil {
		return user.User{}, err
	}
	if !account.IsActive() {
		return user.User{}, auth.ErrUserInactive
	}
	return account, nil
}

func hashPassword(raw string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(raw), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}
