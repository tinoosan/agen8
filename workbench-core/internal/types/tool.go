package types

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type ToolID string
type ActionID string

var toolIDRegex = regexp.MustCompile(`^[a-z0-9]+(\.[a-z0-9_-]+)+$`)
var actionIDRegex = regexp.MustCompile(`^[a-z0-9]+(\.[a-z0-9_-]+)*$`)

func ParseToolID(id string) (ToolID, error) {
	s := strings.ToLower(strings.TrimSpace(id))
	if s == "" {
		return "", fmt.Errorf("tool ID cannot be empty")
	}
	if !toolIDRegex.MatchString(s) {
		return "", fmt.Errorf("invalid tool ID %q (expected dot-separated id like github.com.acme.tool)", s)
	}

	return ToolID(s), nil
}

func (id ToolID) String() string { return string(id) }

func (id ToolID) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(id))
}

func (id *ToolID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := ParseToolID(s)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

func ParseActionID(id string) (ActionID, error) {
	s := strings.ToLower(strings.TrimSpace(id))
	if s == "" {
		return "", fmt.Errorf("action ID cannot be empty")
	}
	if !actionIDRegex.MatchString(s) {
		return "", fmt.Errorf("invalid action ID %q (expected id like exec or workbench.write)", s)
	}
	return ActionID(s), nil
}

func (id ActionID) String() string { return string(id) }

func (id ActionID) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(id))
}

func (id *ActionID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := ParseActionID(s)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

type EnvVar struct {
	// Required maps env var name -> description.
	Required map[string]string `json:"required,omitempty"`
	// Optional maps env var name -> description.
	Optional map[string]string `json:"optional,omitempty"`
}

type ToolKind string

const (
	ToolKindBuiltin ToolKind = "builtin"
	ToolKindCustom  ToolKind = "custom"
)

// ParseUserToolManifest parses and validates a tool manifest provided by a user.
//
// Policy:
//   - If kind is omitted, it defaults to "custom".
//   - "builtin" is reserved for internal manifests and is rejected.
func ParseUserToolManifest(b []byte) (ToolManifest, error) {
	var m ToolManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return ToolManifest{}, err
	}
	if m.Kind == "" {
		m.Kind = ToolKindCustom
	}
	if m.Kind == ToolKindBuiltin {
		return ToolManifest{}, fmt.Errorf("tool kind %q is reserved", ToolKindBuiltin)
	}
	return m, m.Validate()
}

// ParseBuiltinToolManifest parses and validates an internal/built-in tool manifest.
//
// Policy:
//   - If kind is omitted, it defaults to "builtin".
//   - The parsed manifest must be "builtin".
func ParseBuiltinToolManifest(b []byte) (ToolManifest, error) {
	var m ToolManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return ToolManifest{}, err
	}
	if m.Kind == "" {
		m.Kind = ToolKindBuiltin
	}
	if m.Kind != ToolKindBuiltin {
		return ToolManifest{}, fmt.Errorf("builtin tool manifest must have kind %q (got %q)", ToolKindBuiltin, m.Kind)
	}
	return m, m.Validate()
}

type ToolManifest struct {
	ID          ToolID       `json:"id"`
	Version     string       `json:"version"`
	Kind        ToolKind     `json:"kind"` // builtin or custom
	DisplayName string       `json:"displayName"`
	Description string       `json:"description"`
	SourceRepo  string       `json:"sourceRepo,omitempty"`
	Actions     []ToolAction `json:"actions"`
	Env         EnvVar       `json:"env,omitempty"`
}

type ToolAction struct {
	ID           ActionID        `json:"id"` // eg. workbench.write
	DisplayName  string          `json:"displayName"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema"`
}

func (m ToolManifest) Validate() error {
	if _, err := ParseToolID(m.ID.String()); err != nil {
		return err
	}
	if m.Version == "" {
		return fmt.Errorf("tool version cannot be empty")
	}
	switch m.Kind {
	case ToolKindBuiltin, ToolKindCustom:
	default:
		return fmt.Errorf("invalid tool kind %q (expected %q or %q)", m.Kind, ToolKindBuiltin, ToolKindCustom)
	}
	if m.DisplayName == "" {
		return fmt.Errorf("tool displayName cannot be empty")
	}
	if m.Description == "" {
		return fmt.Errorf("tool description cannot be empty")
	}
	if len(m.Actions) == 0 {
		return fmt.Errorf("tool actions cannot be empty")
	}
	seen := make(map[ActionID]struct{}, len(m.Actions))
	for _, a := range m.Actions {
		if _, err := ParseActionID(a.ID.String()); err != nil {
			return err
		}
		if _, ok := seen[a.ID]; ok {
			return fmt.Errorf("duplicate action ID %q", a.ID)
		}
		seen[a.ID] = struct{}{}
		if a.DisplayName == "" {
			return fmt.Errorf("action displayName cannot be empty")
		}
		if a.Description == "" {
			return fmt.Errorf("action description cannot be empty")
		}
		if len(a.InputSchema) == 0 {
			return fmt.Errorf("action inputSchema cannot be empty")
		}
		if len(a.OutputSchema) == 0 {
			return fmt.Errorf("action outputSchema cannot be empty")
		}
		if !json.Valid(a.InputSchema) {
			return fmt.Errorf("action inputSchema is not valid JSON")
		}
		if !json.Valid(a.OutputSchema) {
			return fmt.Errorf("action outputSchema is not valid JSON")
		}
	}

	return nil
}
