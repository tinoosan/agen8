package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/skills"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// PromptBuilder assembles a minimal prompt context for autonomous runs.
// It intentionally avoids session history and trace summaries.
type PromptBuilder struct {
	FS     *vfs.FS
	Skills *skills.Manager

	MaxMemoryBytes int

	Emit events.EmitFunc
}

// SystemPrompt renders a compact prompt: base + memory.
func (c *PromptBuilder) SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error) {
	if strings.TrimSpace(basePrompt) == "" {
		basePrompt = DefaultSystemPrompt()
	}

	sections := []string{strings.TrimSpace(basePrompt)}
	if c == nil || c.FS == nil {
		return strings.TrimSpace(basePrompt), nil
	}

	maxMem := c.MaxMemoryBytes
	if maxMem == 0 {
		maxMem = 8 * 1024
	}
	// Best-effort: include only today's memory file.
	today := time.Now().Format("2006-01-02") + "-memory.md"
	memPath := "/memory/" + today
	mem := strings.TrimSpace(c.readCap(memPath, maxMem))
	if mem != "" && maxMem != 0 {
		sections = append(sections, "## Memory\n\n"+mem)
	}
	if scripts := strings.TrimSpace(c.skillScriptsManifest()); scripts != "" {
		sections = append(sections, scripts)
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

func (c *PromptBuilder) skillScriptsManifest() string {
	if c == nil || c.Skills == nil {
		return ""
	}
	manifest := c.Skills.ScriptsManifest()
	if len(manifest) == 0 {
		return ""
	}
	lines := make([]string, 0, len(manifest))
	for _, item := range manifest {
		if strings.TrimSpace(item.Skill) == "" || len(item.Scripts) == 0 {
			continue
		}
		scriptNames := make([]string, 0, len(item.Scripts))
		for _, script := range item.Scripts {
			name := strings.TrimSpace(script.Name)
			if name == "" {
				continue
			}
			scriptNames = append(scriptNames, name)
		}
		if len(scriptNames) == 0 {
			continue
		}
		sort.Strings(scriptNames)
		lines = append(lines, fmt.Sprintf("%s: %s", item.Skill, strings.Join(scriptNames, ", ")))
	}
	if len(lines) == 0 {
		return ""
	}
	sort.Strings(lines)
	return "<skill_scripts>\n" + strings.Join(lines, "\n") + "\n</skill_scripts>"
}
