package rpc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	userapp "github.com/tinoosan/agen8/internal/services/user/app"
	user "github.com/tinoosan/agen8/internal/services/user/domain"
)

var rpcNow = time.Date(2026, 5, 17, 14, 0, 0, 0, time.UTC)

type fakeUserRepository struct {
	users map[string]user.User
}

func newFakeUserRepository(records ...user.User) *fakeUserRepository {
	repo := &fakeUserRepository{users: map[string]user.User{}}
	for _, record := range records {
		repo.users[record.ID.String()] = record
	}
	return repo
}

func (r *fakeUserRepository) Get(_ context.Context, id user.ID) (user.User, error) {
	record, ok := r.users[id.String()]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return record, nil
}

func (r *fakeUserRepository) GetByEmail(_ context.Context, email string) (user.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, record := range r.users {
		if record.Email == email {
			return record, nil
		}
	}
	return user.User{}, user.ErrNotFound
}

func (r *fakeUserRepository) FirstActive(context.Context) (user.User, error) {
	for _, record := range r.users {
		if record.IsActive() {
			return record, nil
		}
	}
	return user.User{}, user.ErrNotFound
}

func (r *fakeUserRepository) Count(context.Context) (int, error) {
	return len(r.users), nil
}

func (r *fakeUserRepository) Create(_ context.Context, record user.User) error {
	r.users[record.ID.String()] = record
	return nil
}

func (r *fakeUserRepository) Update(_ context.Context, record user.User) error {
	if _, ok := r.users[record.ID.String()]; !ok {
		return user.ErrNotFound
	}
	r.users[record.ID.String()] = record
	return nil
}

func TestStatusReturnsSetupOpenWithoutIdentity(t *testing.T) {
	handler := newHandlerForTest(t, newFakeUserRepository(), func(context.Context) (Identity, error) {
		return Identity{}, errors.New("no identity")
	})

	result, err := handler.Status(context.Background(), StatusParams{})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !result.SetupOpen {
		t.Fatal("expected setup open")
	}
	if result.User != nil {
		t.Fatalf("user=%#v want nil", result.User)
	}
}

func TestStatusReturnsCurrentUserWhenAuthenticated(t *testing.T) {
	record := userRecord(t, "user-1", user.RoleAdmin)
	handler := newHandlerForTest(t, newFakeUserRepository(record), staticIdentity("user-1", "admin"))

	result, err := handler.Status(context.Background(), StatusParams{})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if result.SetupOpen {
		t.Fatal("expected setup closed")
	}
	if result.User == nil || result.User.ID != "user-1" {
		t.Fatalf("user=%#v want user-1", result.User)
	}
}

func TestGetDefaultsToAuthenticatedUser(t *testing.T) {
	record := userRecord(t, "user-1", user.RoleUser)
	handler := newHandlerForTest(t, newFakeUserRepository(record), staticIdentity("user-1", "user"))

	result, err := handler.Get(context.Background(), GetParams{})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if result.User.ID != "user-1" {
		t.Fatalf("id=%q want user-1", result.User.ID)
	}
}

func TestUpdateProfileUsesAuthenticatedUserOnly(t *testing.T) {
	record := userRecord(t, "user-1", user.RoleUser)
	handler := newHandlerForTest(t, newFakeUserRepository(record), staticIdentity("user-1", "user"))
	email := " NEW@example.COM "
	name := "New Name"

	result, err := handler.UpdateProfile(context.Background(), UpdateProfileParams{
		Email: &email,
		Name:  &name,
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if result.User.Email != "new@example.com" {
		t.Fatalf("email=%q want new@example.com", result.User.Email)
	}
	if result.User.Name != "New Name" {
		t.Fatalf("name=%q want New Name", result.User.Name)
	}
}

func TestUpdateProfileReturnsPreferences(t *testing.T) {
	record := userRecord(t, "user-1", user.RoleUser)
	handler := newHandlerForTest(t, newFakeUserRepository(record), staticIdentity("user-1", "user"))
	preferences := UserPreferencesView{
		Theme:              "forest",
		LastDarkTheme:      "forest",
		LastLightTheme:     "solarized",
		DefaultProjectView: "strategy",
		FontFamily:         "mono",
		FontScale:          17,
	}

	result, err := handler.UpdateProfile(context.Background(), UpdateProfileParams{
		Preferences: &preferences,
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if result.User.Preferences.Theme != "forest" {
		t.Fatalf("theme=%q want forest", result.User.Preferences.Theme)
	}
	if result.User.Preferences.LastLightTheme != "solarized" {
		t.Fatalf("last light theme=%q want solarized", result.User.Preferences.LastLightTheme)
	}
	if result.User.Preferences.DefaultProjectView != "strategy" {
		t.Fatalf("default project view=%q want strategy", result.User.Preferences.DefaultProjectView)
	}
	if result.User.Preferences.FontFamily != "mono" {
		t.Fatalf("font family=%q want mono", result.User.Preferences.FontFamily)
	}
	if result.User.Preferences.FontScale != 17 {
		t.Fatalf("font scale=%d want 17", result.User.Preferences.FontScale)
	}
}

func TestSuspendRequiresAdmin(t *testing.T) {
	record := userRecord(t, "user-1", user.RoleUser)
	handler := newHandlerForTest(t, newFakeUserRepository(record), staticIdentity("user-1", "user"))

	_, err := handler.Suspend(context.Background(), SuspendParams{UserID: "user-1"})
	if err == nil {
		t.Fatal("expected non-admin suspend to fail")
	}
}

func TestAdminCanSuspendUser(t *testing.T) {
	record := userRecord(t, "user-1", user.RoleUser)
	handler := newHandlerForTest(t, newFakeUserRepository(record), staticIdentity("admin-1", "admin"))

	result, err := handler.Suspend(context.Background(), SuspendParams{UserID: "user-1"})
	if err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if result.User.Lifecycle != string(user.LifecycleSuspended) {
		t.Fatalf("lifecycle=%q want suspended", result.User.Lifecycle)
	}
}

func TestUserCanCloseSelf(t *testing.T) {
	record := userRecord(t, "user-1", user.RoleUser)
	handler := newHandlerForTest(t, newFakeUserRepository(record), staticIdentity("user-1", "user"))

	result, err := handler.Close(context.Background(), CloseParams{})
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if result.User.Lifecycle != string(user.LifecycleClosed) {
		t.Fatalf("lifecycle=%q want closed", result.User.Lifecycle)
	}
}

func TestUserCannotCloseAnotherUser(t *testing.T) {
	actor := userRecord(t, "user-1", user.RoleUser)
	other := userRecord(t, "user-2", user.RoleUser)
	handler := newHandlerForTest(t, newFakeUserRepository(actor, other), staticIdentity("user-1", "user"))

	_, err := handler.Close(context.Background(), CloseParams{UserID: "user-2"})
	if err == nil {
		t.Fatal("expected close other user to fail")
	}
}

func newHandlerForTest(t *testing.T, repo user.Repository, identity IdentityProvider) *Handler {
	t.Helper()
	svc, err := userapp.NewService(repo, user.FixedClock{T: rpcNow}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return NewHandler(svc, identity)
}

func staticIdentity(userID string, role string) IdentityProvider {
	return func(context.Context) (Identity, error) {
		return Identity{UserID: userID, Role: role}, nil
	}
}

func userRecord(t *testing.T, rawID string, role user.Role) user.User {
	t.Helper()
	id, err := user.NewID(rawID)
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	record, err := user.New(user.NewInput{
		ID:    id,
		Email: rawID + "@example.com",
		Name:  "Test User",
		Role:  role,
		Now:   rpcNow,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return record
}
