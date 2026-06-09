package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	user "github.com/tinoosan/agen8/internal/services/user/domain"
)

type Service struct {
	users  user.Repository
	clock  user.Clock
	logger *slog.Logger
}

func NewService(users user.Repository, clock user.Clock, logger *slog.Logger) (*Service, error) {
	if users == nil {
		return nil, fmt.Errorf("user repository is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("user clock is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("user logger is required")
	}
	return &Service{
		users:  users,
		clock:  clock,
		logger: logger,
	}, nil
}

func (s *Service) SetupOpen(ctx context.Context) (bool, error) {
	count, err := s.users.Count(ctx)
	if err != nil {
		return false, fmt.Errorf("count users: %w", err)
	}
	return count == 0, nil
}

type SetupFirstUserParams struct {
	Email string
	Name  string
}

type SetupFirstUserResult struct {
	User user.User
}

func (s *Service) SetupFirstUser(ctx context.Context, params SetupFirstUserParams) (SetupFirstUserResult, error) {
	count, err := s.users.Count(ctx)
	if err != nil {
		return SetupFirstUserResult{}, fmt.Errorf("count users: %w", err)
	}
	if count != 0 {
		return SetupFirstUserResult{}, user.ErrSetupClosed
	}
	id, err := user.NewID("user_" + uuid.NewString())
	if err != nil {
		return SetupFirstUserResult{}, fmt.Errorf("generate user id: %w", err)
	}
	record, err := user.New(user.NewInput{
		ID:    id,
		Email: params.Email,
		Name:  params.Name,
		Role:  user.RoleAdmin,
		Now:   s.clock.Now(),
	})
	if err != nil {
		return SetupFirstUserResult{}, err
	}
	if err := s.users.Create(ctx, record); err != nil {
		return SetupFirstUserResult{}, fmt.Errorf("create first user: %w", err)
	}
	s.logger.Info("user created", "user_id", record.ID.String(), "role", string(record.Role), "source", "setup")
	return SetupFirstUserResult{User: record}, nil
}

func (s *Service) Get(ctx context.Context, id user.ID) (user.User, error) {
	if id.IsZero() {
		return user.User{}, fmt.Errorf("user id is required")
	}
	record, err := s.users.Get(ctx, id)
	if err != nil {
		return user.User{}, fmt.Errorf("load user: %w", err)
	}
	return record, nil
}

func (s *Service) GetByEmail(ctx context.Context, email string) (user.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return user.User{}, fmt.Errorf("user email is required")
	}
	record, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return user.User{}, fmt.Errorf("load user by email: %w", err)
	}
	return record, nil
}

func (s *Service) FirstActive(ctx context.Context) (user.User, error) {
	record, err := s.users.FirstActive(ctx)
	if err != nil {
		return user.User{}, fmt.Errorf("load first active user: %w", err)
	}
	return record, nil
}

type UpdateProfileParams struct {
	UserID      user.ID
	Email       *string
	Name        *string
	Preferences *user.Preferences
}

func (s *Service) UpdateProfile(ctx context.Context, params UpdateProfileParams) (user.User, error) {
	loaded, err := s.Get(ctx, params.UserID)
	if err != nil {
		return user.User{}, err
	}
	next := loaded
	if params.Email != nil {
		next.Email = strings.ToLower(strings.TrimSpace(*params.Email))
	}
	if params.Name != nil {
		next.Name = strings.TrimSpace(*params.Name)
	}
	if params.Preferences != nil {
		next.Preferences = mergePreferences(next.Preferences, *params.Preferences)
	}
	if strings.TrimSpace(next.Email) == "" {
		return user.User{}, fmt.Errorf("user email is required")
	}
	if strings.TrimSpace(next.Name) == "" {
		return user.User{}, fmt.Errorf("user name is required")
	}
	next.UpdatedAt = s.clock.Now().UTC()
	if err := s.users.Update(ctx, next); err != nil {
		return user.User{}, fmt.Errorf("update user profile: %w", err)
	}
	s.logger.Info("user profile updated", "user_id", next.ID.String())
	return next, nil
}

var (
	allowedThemes = map[string]struct{}{
		"dark": {}, "midnight": {}, "dim": {}, "nebula": {}, "nord": {}, "rose": {}, "forest": {}, "ember": {},
		"light": {}, "sepia": {}, "solarized": {},
	}
	allowedFontFamilies = map[string]struct{}{
		"inter": {}, "geist": {}, "figtree": {}, "space-grotesk": {}, "atkinson": {},
		"system": {}, "serif": {}, "lora": {}, "fraunces": {}, "mono": {},
	}
	allowedDefaultProjectViews = map[string]struct{}{"dashboard": {}, "strategy": {}}
)

func mergePreferences(current user.Preferences, incoming user.Preferences) user.Preferences {
	next := current
	if validChoice(incoming.Theme, allowedThemes) {
		next.Theme = incoming.Theme
	}
	if validChoice(incoming.LastDarkTheme, allowedThemes) {
		next.LastDarkTheme = incoming.LastDarkTheme
	}
	if validChoice(incoming.LastLightTheme, allowedThemes) {
		next.LastLightTheme = incoming.LastLightTheme
	}
	if validChoice(incoming.DefaultProjectView, allowedDefaultProjectViews) {
		next.DefaultProjectView = incoming.DefaultProjectView
	}
	if validChoice(incoming.FontFamily, allowedFontFamilies) {
		next.FontFamily = incoming.FontFamily
	}
	if incoming.FontScale >= 13 && incoming.FontScale <= 20 {
		next.FontScale = incoming.FontScale
	}
	return next
}

func validChoice(value string, allowed map[string]struct{}) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	_, ok := allowed[value]
	return ok
}

func (s *Service) SuspendUser(ctx context.Context, id user.ID) (user.User, error) {
	loaded, err := s.Get(ctx, id)
	if err != nil {
		return user.User{}, err
	}
	if loaded.Lifecycle == user.LifecycleSuspended {
		return loaded, nil
	}
	if loaded.Lifecycle == user.LifecycleClosed {
		return user.User{}, fmt.Errorf("closed user cannot be suspended")
	}
	loaded.Lifecycle = user.LifecycleSuspended
	loaded.UpdatedAt = s.clock.Now().UTC()
	if err := s.users.Update(ctx, loaded); err != nil {
		return user.User{}, fmt.Errorf("suspend user: %w", err)
	}
	s.logger.Info("user suspended", "user_id", loaded.ID.String())
	return loaded, nil
}

func (s *Service) CloseUser(ctx context.Context, id user.ID) (user.User, error) {
	loaded, err := s.Get(ctx, id)
	if err != nil {
		return user.User{}, err
	}
	if loaded.Lifecycle == user.LifecycleClosed {
		return loaded, nil
	}
	loaded.Lifecycle = user.LifecycleClosed
	loaded.UpdatedAt = s.clock.Now().UTC()
	if err := s.users.Update(ctx, loaded); err != nil {
		return user.User{}, fmt.Errorf("close user: %w", err)
	}
	s.logger.Info("user closed", "user_id", loaded.ID.String())
	return loaded, nil
}
