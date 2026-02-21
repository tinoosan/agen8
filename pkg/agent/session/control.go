package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/events"
)

func normalizeReasoningEffort(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "":
		return "", nil
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return v, nil
	default:
		return "", fmt.Errorf("invalid reasoning effort %q", v)
	}
}

func normalizeReasoningSummary(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "none" {
		v = "off"
	}
	switch v {
	case "":
		return "", nil
	case "off", "auto", "concise", "detailed":
		return v, nil
	default:
		return "", fmt.Errorf("invalid reasoning summary %q", v)
	}
}

// SetModel applies a runtime model change to this session.
func (s *Session) SetModel(ctx context.Context, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model is required")
	}
	s.cfg.Agent.SetModel(model)
	s.emitBestEffort(ctx, events.Event{
		Type:    "control.success",
		Message: "Control request applied",
		Data: map[string]string{
			"command": "set_model",
			"model":   model,
		},
	})
	return nil
}

// SetReasoning applies runtime reasoning effort/summary changes to this session.
func (s *Session) SetReasoning(ctx context.Context, effort, summary string) error {
	if s == nil || s.cfg.Agent == nil {
		return fmt.Errorf("session agent is not configured")
	}
	normalizedEffort, err := normalizeReasoningEffort(effort)
	if err != nil {
		return err
	}
	normalizedSummary, err := normalizeReasoningSummary(summary)
	if err != nil {
		return err
	}
	changed := false
	if normalizedEffort != "" && !strings.EqualFold(strings.TrimSpace(s.cfg.Agent.GetReasoningEffort()), normalizedEffort) {
		s.cfg.Agent.SetReasoningEffort(normalizedEffort)
		changed = true
	}
	if normalizedSummary != "" && !strings.EqualFold(strings.TrimSpace(s.cfg.Agent.GetReasoningSummary()), normalizedSummary) {
		s.cfg.Agent.SetReasoningSummary(normalizedSummary)
		changed = true
	}
	if !changed {
		return nil
	}
	data := map[string]string{
		"command": "set_reasoning",
	}
	if normalizedEffort != "" {
		data["effort"] = normalizedEffort
	}
	if normalizedSummary != "" {
		data["summary"] = normalizedSummary
	}
	s.emitBestEffort(ctx, events.Event{
		Type:    "control.success",
		Message: "Control request applied",
		Data:    data,
	})
	return nil
}

// SwitchProfile swaps the active profile for this session.
func (s *Session) SwitchProfile(ctx context.Context, ref string) error {
	if s.cfg.ResolveProfile == nil {
		return fmt.Errorf("profile switching not configured")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("profile is required")
	}
	p, dir, err := s.cfg.ResolveProfile(ref)
	if err != nil {
		return err
	}
	if err := s.setProfile(p, dir); err != nil {
		return err
	}
	s.startHeartbeats(ctx)
	s.emitBestEffort(ctx, events.Event{
		Type:    "control.success",
		Message: "Control request applied",
		Data: map[string]string{
			"command":                 "switch_profile",
			"profile":                 ref,
			"preservesSessionContext": "true",
		},
	})
	return nil
}

type controlTask struct {
	Type      string         `json:"type"`
	Command   string         `json:"command"`
	Args      map[string]any `json:"args,omitempty"`
	Processed bool           `json:"processed,omitempty"`
	At        string         `json:"processedAt,omitempty"`
	Error     string         `json:"error,omitempty"`
}

func (s *Session) tryHandleControl(ctx context.Context, path string, raw string) (handled bool, err error) {
	var c controlTask
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return false, nil
	}
	if strings.ToLower(strings.TrimSpace(c.Type)) != "control" {
		return false, nil
	}
	if c.Processed {
		return true, nil
	}

	if s.cfg.Events != nil {
		_ = s.cfg.Events.Emit(ctx, events.Event{
			Type:    "control.check",
			Message: "Control task received",
			Data:    map[string]string{"command": strings.TrimSpace(c.Command)},
		})
	}

	cmd := strings.ToLower(strings.TrimSpace(c.Command))
	switch cmd {
	case "switch_profile":
		if s.cfg.ResolveProfile == nil {
			c.Error = "profile switching not configured"
			break
		}
		ref := ""
		if c.Args != nil {
			if v, ok := c.Args["profile"]; ok {
				if str, ok := v.(string); ok {
					ref = strings.TrimSpace(str)
				}
			}
		}
		if ref == "" {
			c.Error = "args.profile is required"
			break
		}
		p, dir, rerr := s.cfg.ResolveProfile(ref)
		if rerr != nil {
			c.Error = rerr.Error()
			break
		}
		if err := s.setProfile(p, dir); err != nil {
			c.Error = err.Error()
			break
		}
		s.startHeartbeats(ctx)
	case "set_model":
		ref := ""
		if c.Args != nil {
			if v, ok := c.Args["model"]; ok {
				if str, ok := v.(string); ok {
					ref = strings.TrimSpace(str)
				}
			}
		}
		if ref == "" {
			c.Error = "args.model is required"
			break
		}
		s.cfg.Agent.SetModel(ref)
	default:
		c.Error = fmt.Sprintf("unknown command %q", cmd)
	}

	c.Processed = true
	c.At = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writeJSON(ctx, path, c); err != nil {
		return true, err
	}
	if s.cfg.Events != nil {
		msg := "Control task applied"
		typ := "control.success"
		if strings.TrimSpace(c.Error) != "" {
			msg = "Control task failed"
			typ = "control.error"
		}
		_ = s.cfg.Events.Emit(ctx, events.Event{
			Type:    typ,
			Message: msg,
			Data: map[string]string{
				"command":                 strings.TrimSpace(c.Command),
				"error":                   strings.TrimSpace(c.Error),
				"preservesSessionContext": fmt.Sprintf("%t", cmd == "switch_profile"),
			},
		})
	}
	return true, nil
}
