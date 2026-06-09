package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	user "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
)

var serviceNow = time.Date(2026, 5, 17, 13, 0, 0, 0, time.UTC)

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
	if _, ok := r.users[record.ID.String()]; ok {
		return errors.New("duplicate user id")
	}
	for _, existing := range r.users {
		if existing.Email == strings.ToLower(strings.TrimSpace(record.Email)) {
			return user.ErrEmailAlreadyExists
		}
	}
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

func TestSetupFirstUserCreatesAdminWhenRepositoryIsEmpty(t *testing.T) {
	svc := newServiceForTest(t, newFakeUserRepository())

	result, err := svc.SetupFirstUser(context.Background(), SetupFirstUserParams{
		Email: " ADMIN@example.COM ",
		Name:  " Admin ",
	})
	if err != nil {
		t.Fatalf("SetupFirstUser: %v", err)
	}
	if result.User.ID.IsZero() {
		t.Fatal("expected generated user id")
	}
	if result.User.Email != "admin@example.com" {
		t.Fatalf("email=%q want admin@example.com", result.User.Email)
	}
	if result.User.Name != "Admin" {
		t.Fatalf("name=%q want Admin", result.User.Name)
	}
	if result.User.Role != user.RoleAdmin {
		t.Fatalf("role=%q want admin", result.User.Role)
	}
	if result.User.Lifecycle != user.LifecycleActive {
		t.Fatalf("lifecycle=%q want active", result.User.Lifecycle)
	}
}

func TestSetupFirstUserClosesAfterFirstAccount(t *testing.T) {
	svc := newServiceForTest(t, newFakeUserRepository(userRecord(t, "user-existing")))

	_, err := svc.SetupFirstUser(context.Background(), SetupFirstUserParams{
		Email: "admin@example.com",
		Name:  "Admin",
	})
	if !errors.Is(err, user.ErrSetupClosed) {
		t.Fatalf("err=%v want ErrSetupClosed", err)
	}
}

func TestSetupOpenReflectsRepositoryCount(t *testing.T) {
	openSvc := newServiceForTest(t, newFakeUserRepository())
	open, err := openSvc.SetupOpen(context.Background())
	if err != nil {
		t.Fatalf("SetupOpen empty: %v", err)
	}
	if !open {
		t.Fatal("expected setup open")
	}

	closedSvc := newServiceForTest(t, newFakeUserRepository(userRecord(t, "user-existing")))
	open, err = closedSvc.SetupOpen(context.Background())
	if err != nil {
		t.Fatalf("SetupOpen existing: %v", err)
	}
	if open {
		t.Fatal("expected setup closed")
	}
}

func TestUpdateProfilePreservesAccountState(t *testing.T) {
	record := userRecord(t, "user-1")
	svc := newServiceForTest(t, newFakeUserRepository(record))
	email := " NEW@example.COM "
	name := " New Name "

	updated, err := svc.UpdateProfile(context.Background(), UpdateProfileParams{
		UserID: record.ID,
		Email:  &email,
		Name:   &name,
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if updated.Email != "new@example.com" {
		t.Fatalf("email=%q want new@example.com", updated.Email)
	}
	if updated.Name != "New Name" {
		t.Fatalf("name=%q want New Name", updated.Name)
	}
	if updated.Role != record.Role {
		t.Fatalf("role changed: %q", updated.Role)
	}
}

func TestUpdateProfileMergesPreferences(t *testing.T) {
	record := userRecord(t, "user-1")
	record.Preferences = user.Preferences{
		Theme:              "dark",
		LastDarkTheme:      "dark",
		LastLightTheme:     "light",
		DefaultProjectView: "dashboard",
		FontFamily:         "inter",
		FontScale:          16,
	}
	svc := newServiceForTest(t, newFakeUserRepository(record))
	preferences := user.Preferences{
		Theme:              "rose",
		DefaultProjectView: "strategy",
		FontFamily:         "lora",
		FontScale:          18,
	}

	updated, err := svc.UpdateProfile(context.Background(), UpdateProfileParams{
		UserID:      record.ID,
		Preferences: &preferences,
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if updated.Email != record.Email || updated.Name != record.Name {
		t.Fatalf("profile changed unexpectedly: %#v", updated)
	}
	if updated.Preferences.Theme != "rose" {
		t.Fatalf("theme=%q want rose", updated.Preferences.Theme)
	}
	if updated.Preferences.LastDarkTheme != "dark" {
		t.Fatalf("last dark theme=%q want dark", updated.Preferences.LastDarkTheme)
	}
	if updated.Preferences.DefaultProjectView != "strategy" {
		t.Fatalf("default project view=%q want strategy", updated.Preferences.DefaultProjectView)
	}
	if updated.Preferences.FontFamily != "lora" {
		t.Fatalf("font family=%q want lora", updated.Preferences.FontFamily)
	}
	if updated.Preferences.FontScale != 18 {
		t.Fatalf("font scale=%d want 18", updated.Preferences.FontScale)
	}
}

func TestSuspendAndCloseUser(t *testing.T) {
	record := userRecord(t, "user-1")
	svc := newServiceForTest(t, newFakeUserRepository(record))

	suspended, err := svc.SuspendUser(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("SuspendUser: %v", err)
	}
	if suspended.Lifecycle != user.LifecycleSuspended {
		t.Fatalf("lifecycle=%q want suspended", suspended.Lifecycle)
	}

	closed, err := svc.CloseUser(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("CloseUser: %v", err)
	}
	if closed.Lifecycle != user.LifecycleClosed {
		t.Fatalf("lifecycle=%q want closed", closed.Lifecycle)
	}
}

func TestSuspendClosedUserFails(t *testing.T) {
	record := userRecord(t, "user-1")
	record.Lifecycle = user.LifecycleClosed
	svc := newServiceForTest(t, newFakeUserRepository(record))

	_, err := svc.SuspendUser(context.Background(), record.ID)
	if err == nil {
		t.Fatal("expected suspend closed user to fail")
	}
}

func newServiceForTest(t *testing.T, repo user.Repository) *Service {
	t.Helper()
	svc, err := NewService(repo, user.FixedClock{T: serviceNow}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func userRecord(t *testing.T, rawID string) user.User {
	t.Helper()
	id, err := user.NewID(rawID)
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	record, err := user.New(user.NewInput{
		ID:    id,
		Email: rawID + "@example.com",
		Name:  "Existing User",
		Role:  user.RoleUser,
		Now:   serviceNow,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return record
}
