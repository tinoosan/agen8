package rules

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLRule is a YAML-defined rule with source line metadata.
type YAMLRule[Key comparable] struct {
	Name      string
	Order     int
	AppliesTo []Key
	Lines     []string
	Line      int
}

// YAMLNameRef references a named rule with source line metadata.
type YAMLNameRef struct {
	Name string
	Line int
}

// YAMLAppendRef describes append lines for a named rule with source line metadata.
type YAMLAppendRef struct {
	Name  string
	Lines []string
	Line  int
}

// YAMLComposeOptions mirrors add/disable/append YAML structure with line metadata.
type YAMLComposeOptions[Key comparable] struct {
	Add     []YAMLRule[Key]
	Disable []YAMLNameRef
	Append  []YAMLAppendRef
}

// DecodeComposeOptionsYAML decodes add/disable/append options from a YAML mapping node.
func DecodeComposeOptionsYAML[Key comparable](node *yaml.Node, parseKey func(string) (Key, error)) (YAMLComposeOptions[Key], error) {
	var out YAMLComposeOptions[Key]
	if node == nil || node.Kind == 0 {
		return out, nil
	}
	if node.Kind != yaml.MappingNode {
		return out, fmt.Errorf("rules must be a mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		key := strings.TrimSpace(keyNode.Value)
		switch key {
		case "add":
			add, err := decodeAddRules(valueNode, parseKey)
			if err != nil {
				return out, err
			}
			out.Add = add
		case "disable":
			disable, err := decodeDisableRules(valueNode)
			if err != nil {
				return out, err
			}
			out.Disable = disable
		case "append":
			appendRefs, err := decodeAppendRules(valueNode)
			if err != nil {
				return out, err
			}
			out.Append = appendRefs
		default:
			return out, fmt.Errorf("rules.%s is not supported", key)
		}
	}
	return out, nil
}

func decodeAddRules[Key comparable](node *yaml.Node, parseKey func(string) (Key, error)) ([]YAMLRule[Key], error) {
	if node.Kind == 0 {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("rules.add must be a list")
	}
	out := make([]YAMLRule[Key], 0, len(node.Content))
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("rules.add entries must be mappings")
		}
		var rule YAMLRule[Key]
		rule.Line = item.Line
		for i := 0; i < len(item.Content); i += 2 {
			keyNode := item.Content[i]
			valueNode := item.Content[i+1]
			switch strings.TrimSpace(keyNode.Value) {
			case "name":
				rule.Name = strings.TrimSpace(valueNode.Value)
			case "order":
				if err := valueNode.Decode(&rule.Order); err != nil {
					return nil, fmt.Errorf("rules.add.name=%q order: %w", rule.Name, err)
				}
			case "appliesTo":
				if valueNode.Kind != yaml.SequenceNode {
					return nil, fmt.Errorf("rules.add.name=%q appliesTo must be a list", rule.Name)
				}
				for _, n := range valueNode.Content {
					parsed, err := parseKey(strings.TrimSpace(n.Value))
					if err != nil {
						return nil, err
					}
					rule.AppliesTo = append(rule.AppliesTo, parsed)
				}
			case "lines":
				if valueNode.Kind != yaml.SequenceNode {
					return nil, fmt.Errorf("rules.add.name=%q lines must be a list", rule.Name)
				}
				for _, n := range valueNode.Content {
					rule.Lines = append(rule.Lines, strings.TrimSpace(n.Value))
				}
			default:
				return nil, fmt.Errorf("rules.add.name=%q field %q is not supported", rule.Name, strings.TrimSpace(keyNode.Value))
			}
		}
		out = append(out, rule)
	}
	return out, nil
}

func decodeDisableRules(node *yaml.Node) ([]YAMLNameRef, error) {
	if node.Kind == 0 {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("rules.disable must be a list")
	}
	out := make([]YAMLNameRef, 0, len(node.Content))
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("rules.disable entries must be strings")
		}
		out = append(out, YAMLNameRef{Name: strings.TrimSpace(item.Value), Line: item.Line})
	}
	return out, nil
}

func decodeAppendRules(node *yaml.Node) ([]YAMLAppendRef, error) {
	if node.Kind == 0 {
		return nil, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("rules.append must be a mapping")
	}
	out := make([]YAMLAppendRef, 0, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		if valueNode.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("rules.append.%s must be a list", strings.TrimSpace(keyNode.Value))
		}
		ref := YAMLAppendRef{Name: strings.TrimSpace(keyNode.Value), Line: keyNode.Line}
		for _, item := range valueNode.Content {
			if item.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("rules.append.%s entries must be strings", ref.Name)
			}
			ref.Lines = append(ref.Lines, strings.TrimSpace(item.Value))
		}
		out = append(out, ref)
	}
	return out, nil
}
