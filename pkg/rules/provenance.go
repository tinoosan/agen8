package rules

import "fmt"

// Source identifies where a rule contribution came from.
type Source struct {
	Kind string
	Path string
	Line int
}

func (s Source) String() string {
	if s.Kind == "" && s.Path == "" {
		return "unknown"
	}
	if s.Line > 0 {
		if s.Path != "" {
			return fmt.Sprintf("%s:%s:%d", s.Kind, s.Path, s.Line)
		}
		return fmt.Sprintf("%s:%d", s.Kind, s.Line)
	}
	if s.Path != "" {
		return fmt.Sprintf("%s:%s", s.Kind, s.Path)
	}
	return s.Kind
}

// Provenance captures one rendered contribution.
type Provenance struct {
	RuleName string
	Order    int
	Locked   bool
	BuiltIn  bool
	Source   Source
	Lines    []string
}

// ComposeResult is the rendered prompt plus contribution metadata.
type ComposeResult struct {
	Prompt     string
	Provenance []Provenance
}
