package domain

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

type ModelEntry struct {
	ID      string
	Efforts []string
	Aliases []string
}

type HarnessEntry struct {
	Kind            string
	Models          []ModelEntry
	PermissionModes []PermissionModeEntry
}

type PermissionModeEntry struct {
	ID                string
	Name              string
	Description       string
	Default           bool
	RequiresConfigRef bool
}

type Catalog struct {
	harnesses map[string]HarnessEntry
}

func NewCatalog(entries ...HarnessEntry) *Catalog {
	c := &Catalog{harnesses: make(map[string]HarnessEntry, len(entries))}
	for _, e := range entries {
		c.harnesses[NormalizeRuntimeKind(e.Kind)] = e
	}
	return c
}

func (c *Catalog) ValidateConfig(kind, model, effort string) error {
	kind = NormalizeRuntimeKind(kind)
	if kind == "" {
		return fmt.Errorf("kind is required")
	}
	entry, ok := c.harnesses[kind]
	if !ok {
		supported := c.SupportedHarnesses()
		return fmt.Errorf("unsupported kind %q; supported: %s", kind, strings.Join(supported, ", "))
	}

	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return fmt.Errorf("model is required")
	}
	resolved := c.resolveModel(entry, model)
	if resolved == "" {
		ids := c.supportedModels(entry)
		return fmt.Errorf("unsupported model %q for harness %q; supported: %s", model, kind, strings.Join(ids, ", "))
	}

	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" {
		return fmt.Errorf("effort is required")
	}
	me := c.findModel(entry, resolved)
	if me == nil {
		return fmt.Errorf("model %q not found", resolved)
	}
	if len(me.Efforts) > 0 && !slices.Contains(me.Efforts, effort) {
		return fmt.Errorf("unsupported effort %q for model %q; supported: %s", effort, resolved, strings.Join(me.Efforts, ", "))
	}

	return nil
}

func (c *Catalog) ValidateRuntimeConfig(kind, model, effort, permissionMode, configRef string) error {
	if err := c.ValidateConfig(kind, model, effort); err != nil {
		return err
	}
	mode, err := c.ResolvePermissionMode(kind, permissionMode)
	if err != nil {
		return err
	}
	if mode.RequiresConfigRef && strings.TrimSpace(configRef) == "" {
		return fmt.Errorf("configRef is required for permission mode %q", mode.ID)
	}
	return nil
}

func (c *Catalog) SupportedHarnesses() []string {
	out := make([]string, 0, len(c.harnesses))
	for k := range c.harnesses {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (c *Catalog) Entries() []HarnessEntry {
	out := make([]HarnessEntry, 0, len(c.harnesses))
	for _, entry := range c.harnesses {
		copied := HarnessEntry{
			Kind:            entry.Kind,
			Models:          make([]ModelEntry, len(entry.Models)),
			PermissionModes: make([]PermissionModeEntry, len(entry.PermissionModes)),
		}
		for i, model := range entry.Models {
			copied.Models[i] = ModelEntry{
				ID:      model.ID,
				Efforts: append([]string(nil), model.Efforts...),
				Aliases: append([]string(nil), model.Aliases...),
			}
		}
		copy(copied.PermissionModes, entry.PermissionModes)
		out = append(out, copied)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Kind < out[j].Kind
	})
	return out
}

func (c *Catalog) PermissionModes(kind string) []PermissionModeEntry {
	entry, ok := c.harnesses[NormalizeRuntimeKind(kind)]
	if !ok {
		return nil
	}
	out := make([]PermissionModeEntry, len(entry.PermissionModes))
	copy(out, entry.PermissionModes)
	return out
}

func (c *Catalog) DefaultPermissionMode(kind string) string {
	kind = NormalizeRuntimeKind(kind)
	if compatibility := c.CompatibilityPermissionMode(kind); compatibility != "" {
		return compatibility
	}
	entry, ok := c.harnesses[kind]
	if !ok {
		return ""
	}
	for _, mode := range entry.PermissionModes {
		if mode.Default {
			return mode.ID
		}
	}
	if len(entry.PermissionModes) == 0 {
		return ""
	}
	return entry.PermissionModes[0].ID
}

func (c *Catalog) CompatibilityPermissionMode(kind string) string {
	switch NormalizeRuntimeKind(kind) {
	case "codex":
		return "codex/full-access"
	case "claude-cli":
		return "claude-code/bypass-permissions"
	default:
		return ""
	}
}

func (c *Catalog) ResolvePermissionMode(kind, permissionMode string) (PermissionModeEntry, error) {
	kind = NormalizeRuntimeKind(kind)
	if kind == "" {
		return PermissionModeEntry{}, fmt.Errorf("kind is required")
	}
	entry, ok := c.harnesses[kind]
	if !ok {
		supported := c.SupportedHarnesses()
		return PermissionModeEntry{}, fmt.Errorf("unsupported kind %q; supported: %s", kind, strings.Join(supported, ", "))
	}
	permissionMode = strings.ToLower(strings.TrimSpace(permissionMode))
	if permissionMode == "" {
		permissionMode = c.DefaultPermissionMode(kind)
	}
	for _, mode := range entry.PermissionModes {
		if mode.ID == permissionMode {
			return mode, nil
		}
	}
	ids := make([]string, 0, len(entry.PermissionModes))
	for _, mode := range entry.PermissionModes {
		ids = append(ids, mode.ID)
	}
	return PermissionModeEntry{}, fmt.Errorf("unsupported permission mode %q for harness %q; supported: %s", permissionMode, kind, strings.Join(ids, ", "))
}

func (c *Catalog) SupportedModels(kind string) []string {
	entry, ok := c.harnesses[NormalizeRuntimeKind(kind)]
	if !ok {
		return nil
	}
	return c.supportedModels(entry)
}

func (c *Catalog) SupportedEfforts(kind, model string) []string {
	entry, ok := c.harnesses[NormalizeRuntimeKind(kind)]
	if !ok {
		return nil
	}
	resolved := c.resolveModel(entry, strings.ToLower(strings.TrimSpace(model)))
	if resolved == "" {
		return nil
	}
	me := c.findModel(entry, resolved)
	if me == nil {
		return nil
	}
	out := make([]string, len(me.Efforts))
	copy(out, me.Efforts)
	return out
}

func (c *Catalog) supportedModels(entry HarnessEntry) []string {
	out := make([]string, len(entry.Models))
	for i, m := range entry.Models {
		out[i] = m.ID
	}
	return out
}

func (c *Catalog) resolveModel(entry HarnessEntry, model string) string {
	for _, m := range entry.Models {
		if m.ID == model {
			return m.ID
		}
		for _, alias := range m.Aliases {
			if alias == model {
				return m.ID
			}
		}
	}
	return ""
}

func (c *Catalog) findModel(entry HarnessEntry, id string) *ModelEntry {
	for i := range entry.Models {
		if entry.Models[i].ID == id {
			return &entry.Models[i]
		}
	}
	return nil
}
