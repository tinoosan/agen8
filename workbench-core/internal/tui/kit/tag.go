package kit

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type TagStyles struct {
	KeyStyle   *lipgloss.Style
	ValueStyle *lipgloss.Style
}

type TagOptions struct {
	Key    string
	Value  string
	Styles TagStyles
}

func RenderTag(opts TagOptions) string {
	if opts.Key == "" && opts.Value == "" {
		return ""
	}

	styles := opts.Styles.withDefaults()

	parts := make([]string, 0, 2)
	if opts.Key != "" {
		parts = append(parts, styles.KeyStyle.Render(opts.Key))
	}
	if opts.Value != "" {
		parts = append(parts, styles.ValueStyle.Render(opts.Value))
	}

	return strings.Join(parts, " ")
}

func (s TagStyles) withDefaults() TagStyles {
	if s.KeyStyle == nil {
		s.KeyStyle = CloneStyle(StyleStatusKey)
	}
	if s.ValueStyle == nil {
		s.ValueStyle = CloneStyle(StyleStatusValue)
	}
	return s
}
