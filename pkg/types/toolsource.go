package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// SourceType identifies the kind of external tool source.
type SourceType string

const (
	SourceTypeMCP SourceType = "mcp"
)

var validSourceTypes = map[SourceType]struct{}{
	SourceTypeMCP: {},
}

type ToolSourceOwnerKind string

const (
	ToolSourceOwnerUser    ToolSourceOwnerKind = "user"
	ToolSourceOwnerProject ToolSourceOwnerKind = "project"
)

var validToolSourceOwnerKinds = map[ToolSourceOwnerKind]struct{}{
	ToolSourceOwnerUser:    {},
	ToolSourceOwnerProject: {},
}

type SourceStatus string

const (
	SourceStatusConnected    SourceStatus = "connected"
	SourceStatusDisconnected SourceStatus = "disconnected"
	SourceStatusNeedsLogin   SourceStatus = "needs_login"
	SourceStatusDegraded     SourceStatus = "degraded"
	SourceStatusError        SourceStatus = "error"
)

var validSourceStatuses = map[SourceStatus]struct{}{
	SourceStatusConnected:    {},
	SourceStatusDisconnected: {},
	SourceStatusNeedsLogin:   {},
	SourceStatusDegraded:     {},
	SourceStatusError:        {},
}

func ValidateSourceStatus(s SourceStatus) error {
	if _, ok := validSourceStatuses[s]; !ok {
		return fmt.Errorf("source status %q is not valid (must be one of: connected, disconnected, needs_login, degraded, error)", s)
	}
	return nil
}

type ToolSourceRecord struct {
	ProjectID       string           `json:"projectId"`
	Config          ToolSourceConfig `json:"config"`
	Status          SourceStatus     `json:"status"`
	ToolCount       int              `json:"toolCount"`
	Fingerprint     string           `json:"fingerprint"`
	LastError       string           `json:"lastError,omitempty"`
	LastConnectedAt *time.Time       `json:"lastConnectedAt,omitempty"`
	CreatedAt       time.Time        `json:"createdAt"`
	UpdatedAt       time.Time        `json:"updatedAt"`
}

type ToolSourceConfig struct {
	ID                  string              `json:"id" yaml:"id"`
	OwnerKind           ToolSourceOwnerKind `json:"ownerKind,omitempty" yaml:"ownerKind,omitempty"`
	Type                SourceType          `json:"type" yaml:"type"`
	MCP                 *MCPConfig          `json:"mcp,omitempty" yaml:"mcp,omitempty"`
	Env                 map[string]string   `json:"env,omitempty" yaml:"env,omitempty"`
	Whitelist           []string            `json:"whitelist,omitempty" yaml:"whitelist,omitempty"`
	Auth                *AuthConfig         `json:"auth,omitempty" yaml:"auth,omitempty"`
	CompensatingActions map[string]string   `json:"compensatingActions,omitempty" yaml:"compensatingActions,omitempty"`
}

type ProjectToolSourceAttachment struct {
	ProjectID string `json:"projectId"`
	SourceID  string `json:"sourceId"`
	Alias     string `json:"alias,omitempty"`
	Enabled   bool   `json:"enabled"`
}

type MCPConfig struct {
	URL     string   `json:"url,omitempty" yaml:"url,omitempty"`
	Command string   `json:"command,omitempty" yaml:"command,omitempty"`
	Args    []string `json:"args,omitempty" yaml:"args,omitempty"`
}

type AuthConfig struct {
	Type             string   `json:"type" yaml:"type"`
	TokenEnv         string   `json:"tokenEnv,omitempty" yaml:"tokenEnv,omitempty"`
	AuthorizationURL string   `json:"authorizationUrl,omitempty" yaml:"authorizationUrl,omitempty"`
	TokenURL         string   `json:"tokenUrl,omitempty" yaml:"tokenUrl,omitempty"`
	RegistrationURL  string   `json:"registrationUrl,omitempty" yaml:"registrationUrl,omitempty"`
	ClientID         string   `json:"clientId,omitempty" yaml:"clientId,omitempty"`
	Scopes           []string `json:"scopes,omitempty" yaml:"scopes,omitempty"`
}

func (c ToolSourceConfig) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("tool source ID is required")
	}
	ownerKind := c.OwnerKind
	if strings.TrimSpace(string(ownerKind)) == "" {
		ownerKind = ToolSourceOwnerProject
	}
	if _, ok := validToolSourceOwnerKinds[ownerKind]; !ok {
		return fmt.Errorf("tool source owner kind %q is not valid (must be one of: user, project)", ownerKind)
	}
	if _, ok := validSourceTypes[c.Type]; !ok {
		return fmt.Errorf("tool source type %q is not valid (must be one of: mcp)", c.Type)
	}
	if c.MCP == nil {
		return fmt.Errorf("tool source %q: mcp config is required for type mcp", c.ID)
	}
	hasURL := strings.TrimSpace(c.MCP.URL) != ""
	hasCommand := strings.TrimSpace(c.MCP.Command) != ""
	if !hasURL && !hasCommand {
		return fmt.Errorf("tool source %q: mcp requires either url (HTTP transport) or command (stdio transport)", c.ID)
	}
	for toolName, actionName := range c.CompensatingActions {
		if strings.TrimSpace(toolName) == "" {
			return fmt.Errorf("tool source %q: compensatingActions key cannot be empty", c.ID)
		}
		if strings.TrimSpace(actionName) == "" {
			return fmt.Errorf("tool source %q: compensating action for %q cannot be empty", c.ID, toolName)
		}
	}
	return nil
}

func (c ToolSourceConfig) Fingerprint() string {
	h := sha256.New()
	h.Write([]byte(c.ID))
	h.Write([]byte{0})
	h.Write([]byte(string(c.normalizedOwnerKind())))
	h.Write([]byte{0})
	h.Write([]byte(string(c.Type)))
	h.Write([]byte{0})

	if c.MCP != nil {
		h.Write([]byte("mcp:"))
		h.Write([]byte(c.MCP.URL))
		h.Write([]byte{0})
		h.Write([]byte(c.MCP.Command))
		h.Write([]byte{0})
		for _, arg := range c.MCP.Args {
			h.Write([]byte(arg))
			h.Write([]byte{0})
		}
	}
	if len(c.Env) > 0 {
		keys := make([]string, 0, len(c.Env))
		for k := range c.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		h.Write([]byte("env:"))
		for _, k := range keys {
			h.Write([]byte(k))
			h.Write([]byte{0})
		}
	}
	if len(c.CompensatingActions) > 0 {
		keys := make([]string, 0, len(c.CompensatingActions))
		for k := range c.CompensatingActions {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		h.Write([]byte("comp:"))
		for _, k := range keys {
			h.Write([]byte(k))
			h.Write([]byte{0})
			h.Write([]byte(c.CompensatingActions[k]))
			h.Write([]byte{0})
		}
	}
	if len(c.Whitelist) > 0 {
		whitelist := append([]string(nil), c.Whitelist...)
		sort.Strings(whitelist)
		h.Write([]byte("whitelist:"))
		for _, item := range whitelist {
			h.Write([]byte(strings.TrimSpace(item)))
			h.Write([]byte{0})
		}
	}
	if c.Auth != nil {
		h.Write([]byte("auth:"))
		h.Write([]byte(c.Auth.Type))
		h.Write([]byte{0})
		h.Write([]byte(c.Auth.TokenEnv))
		h.Write([]byte{0})
		h.Write([]byte(c.Auth.AuthorizationURL))
		h.Write([]byte{0})
		h.Write([]byte(c.Auth.TokenURL))
		h.Write([]byte{0})
		h.Write([]byte(c.Auth.RegistrationURL))
		h.Write([]byte{0})
		h.Write([]byte(c.Auth.ClientID))
		h.Write([]byte{0})
		for _, scope := range c.Auth.Scopes {
			h.Write([]byte(scope))
			h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c ToolSourceConfig) normalizedOwnerKind() ToolSourceOwnerKind {
	if strings.TrimSpace(string(c.OwnerKind)) == "" {
		return ToolSourceOwnerProject
	}
	return c.OwnerKind
}
