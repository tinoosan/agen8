package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	decisiontool "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/decision"
	graphtool "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/graph"
	httptool "github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/http"
	"github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/mission"
	"github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/project"
	"github.com/tinoosan/agen8-mcp-server/internal/mcp/tools/task"
)

type toolDef struct {
	native      nativeToolDef
	inputSchema json.RawMessage
}

type nativeToolDef struct {
	name        string
	description string
	schema      json.RawMessage
	internal    bool
}

func buildToolDefs() ([]toolDef, error) {
	natives := []nativeToolDef{
		{name: decisiontool.Name, description: decisiontool.Description, schema: decisiontool.NewHandler().Schema()},
		{name: graphtool.Name, description: graphtool.Description, schema: graphtool.NewHandler().Schema()},
		{name: httptool.Name, description: httptool.Description, schema: httptool.NewHandler().Schema()},
		{name: mission.Name, description: mission.Description, schema: mission.NewHandler().Schema()},
		{name: project.Name, description: project.Description, schema: project.NewHandler().Schema()},
		{name: task.Name, description: task.Description, schema: task.NewHandler().Schema()},
	}
	names := make([]string, 0, len(natives))
	nativeByName := make(map[string]nativeToolDef, len(natives))
	for _, native := range natives {
		name := strings.TrimSpace(native.name)
		if name == "" {
			continue
		}
		nativeByName[name] = native
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]toolDef, 0, len(names))
	for _, name := range names {
		if native, ok := nativeByName[name]; ok {
			schema, err := normalizeToolSchema(native.name, native.schema)
			if err != nil {
				return nil, err
			}
			out = append(out, toolDef{native: native, inputSchema: schema})
			continue
		}
	}
	return out, nil
}

func buildToolDiscoveryCatalog(defs []toolDef) types.ToolDiscoveryCatalog {
	if len(defs) == 0 {
		return types.ToolDiscoveryCatalog{}
	}
	entries := make([]types.ToolDiscoveryEntry, 0, len(defs))
	for _, def := range defs {
		if def.native.internal {
			continue
		}
		name := strings.TrimSpace(def.name())
		if name == "" {
			continue
		}
		entries = append(entries, types.ToolDiscoveryEntry{
			Name:              name,
			Description:       strings.TrimSpace(def.description()),
			DirectAvailable:   true,
			BridgeAvailable:   true,
			PrimaryInvocation: name,
			BridgeCallSyntax:  fmt.Sprintf("%s(args={...})", name),
			Usage:             append([]string(nil), def.usageNotes()...),
			Schema:            append(json.RawMessage(nil), def.inputSchema...),
		})
	}
	return types.ToolDiscoveryCatalog{Tools: entries}
}

func (d toolDef) name() string {
	if strings.TrimSpace(d.native.name) != "" {
		return strings.TrimSpace(d.native.name)
	}
	return ""
}

func (d toolDef) description() string {
	if strings.TrimSpace(d.native.name) != "" {
		return strings.TrimSpace(d.native.description)
	}
	return ""
}

func (d toolDef) usageNotes() []string {
	return nil
}

func normalizeToolSchema(toolName string, raw json.RawMessage) (json.RawMessage, error) {
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) == 0 {
		return nil, fmt.Errorf("mcp tool %q schema is required", strings.TrimSpace(toolName))
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("mcp tool %q schema must be valid JSON", strings.TrimSpace(toolName))
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("mcp tool %q schema decode: %w", strings.TrimSpace(toolName), err)
	}
	typeName, _ := schema["type"].(string)
	if strings.TrimSpace(typeName) != "object" {
		return nil, fmt.Errorf("mcp tool %q schema must have type=object", strings.TrimSpace(toolName))
	}
	return append(json.RawMessage(nil), raw...), nil
}
