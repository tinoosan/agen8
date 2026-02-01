package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/events"
)

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
				"command": strings.TrimSpace(c.Command),
				"error":   strings.TrimSpace(c.Error),
			},
		})
	}
	return true, nil
}
