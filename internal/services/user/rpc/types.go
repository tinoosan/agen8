package rpc

import (
	"time"

	user "github.com/tinoosan/agen8/internal/services/user/domain"
)

type UserView struct {
	ID          string              `json:"id"`
	Email       string              `json:"email"`
	Name        string              `json:"name"`
	Role        string              `json:"role"`
	Lifecycle   string              `json:"lifecycle"`
	Preferences UserPreferencesView `json:"preferences"`
	CreatedAt   *time.Time          `json:"createdAt,omitempty"`
	UpdatedAt   *time.Time          `json:"updatedAt,omitempty"`
}

func NewUserView(record user.User) UserView {
	return UserView{
		ID:          record.ID.String(),
		Email:       record.Email,
		Name:        record.Name,
		Role:        string(record.Role),
		Lifecycle:   string(record.Lifecycle),
		Preferences: newUserPreferencesView(record.Preferences),
		CreatedAt:   cloneTime(record.CreatedAt),
		UpdatedAt:   cloneTime(record.UpdatedAt),
	}
}

type UserPreferencesView struct {
	Theme              string `json:"theme,omitempty"`
	LastDarkTheme      string `json:"lastDarkTheme,omitempty"`
	LastLightTheme     string `json:"lastLightTheme,omitempty"`
	DefaultProjectView string `json:"defaultProjectView,omitempty"`
	FontFamily         string `json:"fontFamily,omitempty"`
	FontScale          int    `json:"fontScale,omitempty"`
}

func newUserPreferencesView(preferences user.Preferences) UserPreferencesView {
	return UserPreferencesView{
		Theme:              preferences.Theme,
		LastDarkTheme:      preferences.LastDarkTheme,
		LastLightTheme:     preferences.LastLightTheme,
		DefaultProjectView: preferences.DefaultProjectView,
		FontFamily:         preferences.FontFamily,
		FontScale:          preferences.FontScale,
	}
}

func (p UserPreferencesView) domain() user.Preferences {
	return user.Preferences{
		Theme:              p.Theme,
		LastDarkTheme:      p.LastDarkTheme,
		LastLightTheme:     p.LastLightTheme,
		DefaultProjectView: p.DefaultProjectView,
		FontFamily:         p.FontFamily,
		FontScale:          p.FontScale,
	}
}

type StatusParams struct{}

type StatusResult struct {
	SetupOpen bool      `json:"setupOpen"`
	User      *UserView `json:"user,omitempty"`
}

type GetParams struct {
	UserID string `json:"userId,omitempty"`
}

type GetResult struct {
	User UserView `json:"user"`
}

type UpdateProfileParams struct {
	Email       *string              `json:"email,omitempty"`
	Name        *string              `json:"name,omitempty"`
	Preferences *UserPreferencesView `json:"preferences,omitempty"`
}

type UpdateProfileResult struct {
	User UserView `json:"user"`
}

type SuspendParams struct {
	UserID string `json:"userId"`
}

type SuspendResult struct {
	User UserView `json:"user"`
}

type CloseParams struct {
	UserID string `json:"userId,omitempty"`
}

type CloseResult struct {
	User UserView `json:"user"`
}

func cloneTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
