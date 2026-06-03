package protocol

// AuthRegisterParams are the params for auth.register.
type AuthRegisterParams struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// AuthRegisterResult is the result for auth.register.
type AuthRegisterResult struct {
	User  AuthUser `json:"user"`
	Token string   `json:"token,omitempty"`
}

// AuthLoginParams are the params for auth.login.
type AuthLoginParams struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthLoginResult is the result for auth.login.
type AuthLoginResult struct {
	User  AuthUser `json:"user"`
	Token string   `json:"token,omitempty"`
}

// AuthLogoutResult is the result for auth.logout.
type AuthLogoutResult struct{}

// AuthProfileUpdateParams are the params for auth.profile.update.
type AuthProfileUpdateParams struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// AuthProfileUpdateResult is the result for auth.profile.update.
type AuthProfileUpdateResult struct {
	User AuthUser `json:"user"`
}

// AuthPasswordChangeParams are the params for auth.password.change.
type AuthPasswordChangeParams struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// AuthPasswordChangeResult is the result for auth.password.change.
type AuthPasswordChangeResult struct{}

// AuthStatusResult is the result for auth.status.
type AuthStatusResult struct {
	Enabled          bool                  `json:"enabled"`
	HostedMode       bool                  `json:"hostedMode"`
	Authenticated    bool                  `json:"authenticated"`
	RegistrationOpen bool                  `json:"registrationOpen"`
	User             *AuthUser             `json:"user,omitempty"`
	Bridge           *BridgeConnectionInfo `json:"bridge,omitempty"`
}

// AuthAPIKeyCreateParams are the params for auth.apikey.create.
type AuthAPIKeyCreateParams struct {
	Name string `json:"name"`
}

// AuthAPIKeyCreateResult is the result for auth.apikey.create.
type AuthAPIKeyCreateResult struct {
	Key    AuthAPIKeyInfo `json:"key"`
	Secret string         `json:"secret"`
}

// AuthAPIKeyListResult is the result for auth.apikey.list.
type AuthAPIKeyListResult struct {
	Keys []AuthAPIKeyInfo `json:"keys"`
}

// AuthAPIKeyRevokeParams are the params for auth.apikey.revoke.
type AuthAPIKeyRevokeParams struct {
	KeyID string `json:"keyId"`
}

// AuthAPIKeyRevokeResult is the result for auth.apikey.revoke.
type AuthAPIKeyRevokeResult struct{}

// AuthUser is the wire format for an authenticated user.
type AuthUser struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

// AuthAPIKeyInfo is the wire format for an API key listing.
type AuthAPIKeyInfo struct {
	ID        string `json:"id"`
	Prefix    string `json:"prefix"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	LastUsed  string `json:"lastUsed,omitempty"`
}

// BridgeConnectionInfo is the hosted bridge connection state for the current user.
type BridgeConnectionInfo struct {
	Connected    bool            `json:"connected"`
	ConnectionID string          `json:"connectionId,omitempty"`
	Status       string          `json:"status,omitempty"`
	ConnectedAt  string          `json:"connectedAt,omitempty"`
	LastSeenAt   string          `json:"lastSeenAt,omitempty"`
	AgentVersion string          `json:"agentVersion,omitempty"`
	Platform     string          `json:"platform,omitempty"`
	Projects     []BridgeProject `json:"projects,omitempty"`
}

type BridgeProject struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Name      string `json:"name,omitempty"`
	SpaceID   string `json:"spaceId,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}
