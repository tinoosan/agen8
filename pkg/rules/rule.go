package rules

import (
	"fmt"
	"strings"
)

// BuildFunc renders dynamic rule content from context.
type BuildFunc[Ctx any] func(ctx Ctx) string

// Rule defines one composable policy block.
// Exactly one of Lines or Build must be set.
type Rule[Ctx any, Key comparable] struct {
	Name      string
	Order     int
	AppliesTo []Key
	Locked    bool
	Lines     []string
	Build     BuildFunc[Ctx]
}

// Validate ensures the rule is structurally valid.
func (r Rule[Ctx, Key]) Validate() error {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return fmt.Errorf("rule name is required")
	}
	if len(r.AppliesTo) == 0 {
		return fmt.Errorf("rule %q applies_to is required", name)
	}
	hasBuild := r.Build != nil
	hasLines := len(r.normalizedLines()) > 0
	if hasBuild && hasLines {
		return fmt.Errorf("rule %q must not set both lines and build", name)
	}
	if !hasBuild && !hasLines {
		return fmt.Errorf("rule %q must set one of lines or build", name)
	}
	return nil
}

func (r Rule[Ctx, Key]) normalizedLines() []string {
	if len(r.Lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(r.Lines))
	for _, line := range r.Lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r Rule[Ctx, Key]) appliesTo(key Key) bool {
	for _, item := range r.AppliesTo {
		if item == key {
			return true
		}
	}
	return false
}

func (r Rule[Ctx, Key]) clone() Rule[Ctx, Key] {
	out := r
	if len(r.AppliesTo) > 0 {
		out.AppliesTo = append([]Key(nil), r.AppliesTo...)
	}
	if len(r.Lines) > 0 {
		out.Lines = append([]string(nil), r.Lines...)
	}
	return out
}

func (r Rule[Ctx, Key]) renderBase(ctx Ctx) string {
	if r.Build != nil {
		return r.Build(ctx)
	}
	return joinLines(r.normalizedLines())
}

func normalizeRuleName(name string) string {
	return strings.TrimSpace(name)
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderedLines(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	parts := strings.Split(text, "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
