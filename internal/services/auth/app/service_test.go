package app

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/apikey"
	auth "github.com/tinoosan/agen8-mcp-server/internal/services/auth/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/password"
	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/session"
	user "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
)

var authTestNow = time.Date(2026, 5, 17, 16, 0, 0, 0, time.UTC)

func TestCreatePasswordStoresHashAndVerifyPasswordAcceptsRawPassword(t *testing.T) {
	account := authUserRecord(t, "user-1", user.LifecycleActive)
	svc, deps := newAuthServiceForTest(t, account)

	if err := svc.CreatePassword(context.Background(), CreatePasswordParams{
		UserID:   account.ID,
		Password: "valid-password",
	}); err != nil {
		t.Fatalf("CreatePassword: %v", err)
	}
	credential, err := deps.passwords.Get(context.Background(), account.ID)
	if err != nil {
		t.Fatalf("password Get: %v", err)
	}
	if credential.PasswordHash == "valid-password" {
		t.Fatal("stored password hash must not equal raw password")
	}
	if err := svc.VerifyPassword(context.Background(), account.ID, "valid-password"); err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if err := svc.VerifyPassword(context.Background(), account.ID, "wrong-password"); err != auth.ErrInvalidCredential {
		t.Fatalf("VerifyPassword wrong password error=%v want invalid credential", err)
	}
}

func TestCreateSessionRequiresActiveUser(t *testing.T) {
	account := authUserRecord(t, "user-1", user.LifecycleSuspended)
	svc, _ := newAuthServiceForTest(t, account)

	_, err := svc.CreateSession(context.Background(), CreateSessionParams{UserID: account.ID})
	if err != auth.ErrUserInactive {
		t.Fatalf("CreateSession error=%v want inactive user", err)
	}
}

func TestCreateAndValidateSessionReturnsUser(t *testing.T) {
	account := authUserRecord(t, "user-1", user.LifecycleActive)
	svc, _ := newAuthServiceForTest(t, account)

	result, err := svc.CreateSession(context.Background(), CreateSessionParams{UserID: account.ID})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if result.Token == "" {
		t.Fatal("session token is required")
	}
	got, err := svc.ValidateSession(context.Background(), result.Token)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if got.ID != account.ID {
		t.Fatalf("user id=%q want %q", got.ID.String(), account.ID.String())
	}
}

func TestLoginVerifiesPasswordAndCreatesSession(t *testing.T) {
	account := authUserRecord(t, "user-1", user.LifecycleActive)
	svc, _ := newAuthServiceForTest(t, account)
	if err := svc.CreatePassword(context.Background(), CreatePasswordParams{
		UserID:   account.ID,
		Password: "valid-password",
	}); err != nil {
		t.Fatalf("CreatePassword: %v", err)
	}

	result, err := svc.Login(context.Background(), LoginParams{
		Email:    " USER-1@example.COM ",
		Password: "valid-password",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.User.ID != account.ID {
		t.Fatalf("user id=%q want %q", result.User.ID.String(), account.ID.String())
	}
	if result.Token == "" {
		t.Fatal("login token is required")
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	account := authUserRecord(t, "user-1", user.LifecycleActive)
	svc, _ := newAuthServiceForTest(t, account)
	if err := svc.CreatePassword(context.Background(), CreatePasswordParams{
		UserID:   account.ID,
		Password: "valid-password",
	}); err != nil {
		t.Fatalf("CreatePassword: %v", err)
	}

	_, err := svc.Login(context.Background(), LoginParams{
		Email:    "user-1@example.com",
		Password: "wrong-password",
	})
	if err != auth.ErrInvalidCredential {
		t.Fatalf("Login error=%v want invalid credential", err)
	}
}

func TestCreateAPIKeyRequiresName(t *testing.T) {
	account := authUserRecord(t, "user-1", user.LifecycleActive)
	svc, _ := newAuthServiceForTest(t, account)

	_, err := svc.CreateAPIKey(context.Background(), CreateAPIKeyParams{UserID: account.ID})
	if err == nil {
		t.Fatal("expected missing api key name to fail")
	}
}

func TestCredentialLifecycleLogsOmitCredentialIdentifiersAndTokens(t *testing.T) {
	account := authUserRecord(t, "user-1", user.LifecycleActive)
	var logOutput bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logOutput, nil))
	svc, _ := newAuthServiceForTestWithLogger(t, logger, account)

	sessionResult, err := svc.CreateSession(context.Background(), CreateSessionParams{UserID: account.ID})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	apiKeyResult, err := svc.CreateAPIKey(context.Background(), CreateAPIKeyParams{
		UserID: account.ID,
		Name:   "ci key",
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	logs := logOutput.String()
	if !strings.Contains(logs, "auth session created") {
		t.Fatalf("logs missing session creation event: %s", logs)
	}
	if !strings.Contains(logs, "auth api key created") {
		t.Fatalf("logs missing api key creation event: %s", logs)
	}
	for _, sensitive := range []string{
		sessionResult.Session.ID.String(),
		sessionResult.Token,
		apiKeyResult.APIKey.ID.String(),
		apiKeyResult.APIKey.Name,
		apiKeyResult.Token,
		apiKeyResult.APIKey.Prefix,
	} {
		if strings.Contains(logs, sensitive) {
			t.Fatalf("auth lifecycle logs exposed sensitive value %q in %s", sensitive, logs)
		}
	}
}

type authTestDeps struct {
	passwords *memoryPasswordRepo
	sessions  *memorySessionRepo
	apiKeys   *memoryAPIKeyRepo
	users     *memoryUserLoader
}

func newAuthServiceForTest(t *testing.T, users ...user.User) (*Service, authTestDeps) {
	t.Helper()
	return newAuthServiceForTestWithLogger(t, slog.New(slog.NewTextHandler(io.Discard, nil)), users...)
}

func newAuthServiceForTestWithLogger(t *testing.T, logger *slog.Logger, users ...user.User) (*Service, authTestDeps) {
	t.Helper()
	deps := authTestDeps{
		passwords: &memoryPasswordRepo{records: map[string]password.Credential{}},
		sessions:  &memorySessionRepo{records: map[string]session.Session{}},
		apiKeys:   &memoryAPIKeyRepo{records: map[string]apikey.Key{}},
		users:     newMemoryUserLoader(users...),
	}
	svc, err := NewService(
		deps.passwords,
		deps.sessions,
		deps.apiKeys,
		deps.users,
		auth.FixedClock{T: authTestNow},
		logger,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, deps
}

type memoryUserLoader struct {
	users map[string]user.User
}

func newMemoryUserLoader(records ...user.User) *memoryUserLoader {
	loader := &memoryUserLoader{users: map[string]user.User{}}
	for _, record := range records {
		loader.users[record.ID.String()] = record
	}
	return loader
}

func (l *memoryUserLoader) Get(_ context.Context, id user.ID) (user.User, error) {
	record, ok := l.users[id.String()]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return record, nil
}

func (l *memoryUserLoader) GetByEmail(_ context.Context, email string) (user.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, record := range l.users {
		if record.Email == email {
			return record, nil
		}
	}
	return user.User{}, user.ErrNotFound
}

type memoryPasswordRepo struct {
	records map[string]password.Credential
}

func (r *memoryPasswordRepo) Get(_ context.Context, userID user.ID) (password.Credential, error) {
	record, ok := r.records[userID.String()]
	if !ok {
		return password.Credential{}, auth.ErrTokenNotFound
	}
	return record, nil
}

func (r *memoryPasswordRepo) Save(_ context.Context, credential password.Credential) error {
	r.records[credential.UserID.String()] = credential
	return nil
}

func (r *memoryPasswordRepo) Delete(_ context.Context, userID user.ID) error {
	delete(r.records, userID.String())
	return nil
}

type memorySessionRepo struct {
	records map[string]session.Session
}

func (r *memorySessionRepo) GetByTokenHash(_ context.Context, tokenHash string) (session.Session, error) {
	record, ok := r.records[tokenHash]
	if !ok {
		return session.Session{}, auth.ErrTokenNotFound
	}
	return record, nil
}

func (r *memorySessionRepo) Create(_ context.Context, record session.Session) error {
	r.records[record.TokenHash] = record
	return nil
}

func (r *memorySessionRepo) Update(_ context.Context, record session.Session) error {
	r.records[record.TokenHash] = record
	return nil
}

type memoryAPIKeyRepo struct {
	records map[string]apikey.Key
}

func (r *memoryAPIKeyRepo) GetByTokenHash(_ context.Context, tokenHash string) (apikey.Key, error) {
	record, ok := r.records[tokenHash]
	if !ok {
		return apikey.Key{}, auth.ErrTokenNotFound
	}
	return record, nil
}

func (r *memoryAPIKeyRepo) Get(_ context.Context, id apikey.ID) (apikey.Key, error) {
	for _, record := range r.records {
		if record.ID == id {
			return record, nil
		}
	}
	return apikey.Key{}, auth.ErrTokenNotFound
}

func (r *memoryAPIKeyRepo) Create(_ context.Context, record apikey.Key) error {
	r.records[record.TokenHash] = record
	return nil
}

func (r *memoryAPIKeyRepo) Update(_ context.Context, record apikey.Key) error {
	r.records[record.TokenHash] = record
	return nil
}

func authUserRecord(t *testing.T, rawID string, lifecycle user.Lifecycle) user.User {
	t.Helper()
	id, err := user.NewID(rawID)
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	record, err := user.New(user.NewInput{
		ID:    id,
		Email: rawID + "@example.com",
		Name:  "Test User",
		Role:  user.RoleUser,
		Now:   authTestNow,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	record.Lifecycle = lifecycle
	return record
}
