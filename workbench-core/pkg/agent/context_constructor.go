package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/skills"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// PromptBuilder assembles a minimal prompt context for autonomous runs.
// It intentionally avoids session history, trace summaries, and skill metadata.
type PromptBuilder struct {
	FS *vfs.FS

	Cfg config.Config

	RunID     string
	SessionID string

	LoadSession func(sessionID string) (types.Session, error)
	SaveSession func(session types.Session) error
	StateStore  store.ConstructorStateStore

	Trace         *TraceMiddleware
	HistoryStore  store.HistoryReader
	SkillsManager *skills.Manager

	IncludeHistoryOps bool

	MaxProfileBytes int
	MaxMemoryBytes  int
	MaxTraceBytes   int
	MaxHistoryBytes int

	TraceIncludeTypes []string

	Emit events.EmitFunc

	LastOp      *types.HostOpRequest
	LastResp    *types.HostOpResponse
	LastToolRun *LastToolRun

	FileAttachments []FileAttachment
}

	// SystemPrompt renders a compact prompt: base + user_profile + memory.
func (c *PromptBuilder) SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error) {
	if strings.TrimSpace(basePrompt) == "" {
		basePrompt = DefaultSystemPrompt()
	}
	_ = ctx

	sections := []string{strings.TrimSpace(basePrompt)}
	if c == nil || c.FS == nil {
		return strings.TrimSpace(basePrompt), nil
	}

	if c.MaxProfileBytes != 0 {
		if profile := c.readCap("/user_profile/user_profile.md", c.MaxProfileBytes); profile != "" {
			sections = append(sections, "## User Profile\n\n"+profile)
		} else if legacy := c.readCap("/profile/profile.md", c.MaxProfileBytes); legacy != "" {
			sections = append(sections, "## User Profile\n\n"+legacy)
		}
	}
	if c.MaxMemoryBytes != 0 {
		todayPath := "/memory/" + time.Now().Format("2006-01-02") + "-memory.md"
		if memory := c.readCap(todayPath, c.MaxMemoryBytes); memory != "" {
			sections = append(sections, "## Memory\n\n"+memory)
		}
	}

	out := strings.TrimSpace(strings.Join(nonEmpty(sections), "\n\n")) + "\n"
	if c.Emit != nil {
		c.Emit(ctx, events.Event{
			Type:    "context.constructor",
			Message: "Context constructor updated",
			Origin:  "env",
			Data: map[string]string{
				"step": fmt.Sprintf("%d", step),
			},
		})
	}
	return out, nil
}

// ObserveHostOp records the most recent host op request/response.
func (c *PromptBuilder) ObserveHostOp(req types.HostOpRequest, resp types.HostOpResponse) {
	if c == nil {
		return
	}
	reqCopy := req
	respCopy := resp
	c.LastOp = &reqCopy
	c.LastResp = &respCopy
	if req.Op == types.HostOpToolRun && resp.ToolResponse != nil {
		c.LastToolRun = &LastToolRun{
			ToolID:   req.ToolID,
			ActionID: req.ActionID,
			CallID:   resp.ToolResponse.CallID,
		}
	}
}

// SetFileAttachments replaces the current attachment set for this turn.
func (c *PromptBuilder) SetFileAttachments(attachments []FileAttachment) {
	if c == nil {
		return
	}
	c.FileAttachments = attachments
}

// ClearFileAttachments clears any cached attachments for the current turn.
func (c *PromptBuilder) ClearFileAttachments() {
	if c == nil {
		return
	}
	c.FileAttachments = nil
}

func (c *PromptBuilder) readCap(path string, maxBytes int) string {
	b, err := c.FS.Read(path)
	if err != nil {
		return ""
	}
	if maxBytes <= 0 || len(b) <= maxBytes {
		return string(b)
	}
	return string(b[:maxBytes])
}

func nonEmpty(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}
