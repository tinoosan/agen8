package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/skills"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// ContextConstructor assembles a minimal prompt context for autonomous runs.
// It intentionally avoids session history, trace summaries, and skill metadata.
type ContextConstructor struct {
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

	Emit func(eventType, message string, data map[string]string)

	LastOp      *types.HostOpRequest
	LastResp    *types.HostOpResponse
	LastToolRun *LastToolRun

	FileAttachments []FileAttachment
}

// SystemPrompt renders a compact prompt: base + profile + memory.
func (c *ContextConstructor) SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error) {
	if strings.TrimSpace(basePrompt) == "" {
		basePrompt = DefaultSystemPrompt()
	}
	_ = ctx

	sections := []string{strings.TrimSpace(basePrompt)}
	if c == nil || c.FS == nil {
		return strings.TrimSpace(basePrompt), nil
	}

	if c.MaxProfileBytes != 0 {
		if profile := c.readCap("/profile/profile.md", c.MaxProfileBytes); profile != "" {
			sections = append(sections, "## Profile\n\n"+profile)
		}
	}
	if c.MaxMemoryBytes != 0 {
		if memory := c.readCap("/memory/memory.md", c.MaxMemoryBytes); memory != "" {
			sections = append(sections, "## Memory\n\n"+memory)
		}
	}

	out := strings.TrimSpace(strings.Join(nonEmpty(sections), "\n\n")) + "\n"
	if c.Emit != nil {
		c.Emit("context.constructor", "Context constructor updated", map[string]string{
			"step": fmt.Sprintf("%d", step),
		})
	}
	return out, nil
}

// ObserveHostOp records the most recent host op request/response.
func (c *ContextConstructor) ObserveHostOp(req types.HostOpRequest, resp types.HostOpResponse) {
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
func (c *ContextConstructor) SetFileAttachments(attachments []FileAttachment) {
	if c == nil {
		return
	}
	c.FileAttachments = attachments
}

// ClearFileAttachments clears any cached attachments for the current turn.
func (c *ContextConstructor) ClearFileAttachments() {
	if c == nil {
		return
	}
	c.FileAttachments = nil
}

func (c *ContextConstructor) readCap(path string, maxBytes int) string {
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
