package builtins

import (
	"context"
	"fmt"
	"sort"

	"github.com/tinoosan/workbench-core/pkg/store"
	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
)

// BuiltinConfig contains host-provided configuration used when constructing builtin invokers.
type BuiltinConfig struct {
	ShellRootDir string
	ShellVFSMount string
	ShellConfirm func(ctx context.Context, argv []string, cwd string) (bool, error)
	TraceStore store.TraceStore
}

// BuiltinDef describes a builtin tool definition: manifest bytes + an optional invoker factory.
type BuiltinDef struct {
	ID         pkgtools.ToolID
	Manifest   []byte
	NewInvoker func(cfg BuiltinConfig) pkgtools.ToolInvoker
}

var builtinDefs []BuiltinDef

// registerBuiltin registers a builtin tool definition into the tools package registry.
func registerBuiltin(def BuiltinDef) {
	if def.ID.String() == "" {
		panic("builtin tool id is required")
	}
	if len(def.Manifest) == 0 {
		panic(fmt.Sprintf("builtin tool %q manifest is required", def.ID.String()))
	}
	for _, existing := range builtinDefs {
		if existing.ID == def.ID {
			panic(fmt.Sprintf("duplicate builtin tool id %q", def.ID.String()))
		}
	}
	builtinDefs = append(builtinDefs, def)
}

// BuiltinInvokerRegistry constructs an in-memory ToolRegistry for executable builtins.
func BuiltinInvokerRegistry(cfg BuiltinConfig) pkgtools.MapRegistry {
	out := make(pkgtools.MapRegistry)

	defs := make([]BuiltinDef, len(builtinDefs))
	copy(defs, builtinDefs)
	sort.Slice(defs, func(i, j int) bool { return defs[i].ID.String() < defs[j].ID.String() })

	for _, def := range defs {
		if def.NewInvoker == nil {
			continue
		}
		out[def.ID] = def.NewInvoker(cfg)
	}
	return out
}
