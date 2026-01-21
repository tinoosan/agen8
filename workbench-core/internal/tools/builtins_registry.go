package tools

import (
	"context"
	"fmt"
	"sort"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/types"
)

// BuiltinConfig contains host-provided configuration used when constructing builtin invokers.
//
// Builtins are discovered via /tools (manifest bytes), but executed via tool.run which requires
// real host configuration (filesystem roots, caps, env, etc). BuiltinConfig is where the host
// supplies those runtime knobs.
type BuiltinConfig struct {
	// ShellRootDir is the OS directory used as the sandbox root for builtin.shell.
	// It must be an absolute path; builtin.shell rejects cwd escapes and absolute cwd.
	ShellRootDir string
	// ShellVFSMount is the VFS path prefix (mount name) used for builtin.shell.
	// When set, stdout/stderr paths are rewritten from the host root into /{mount},
	// and argv elements that begin with the mount prefix are translated back to host paths.
	ShellVFSMount string
	// ShellConfirm is an optional host callback to confirm execution.
	ShellConfirm func(ctx context.Context, argv []string, cwd string) (bool, error)

	// TraceStore is the run-scoped trace store used by builtin.trace.
	TraceStore store.TraceStore
}

// BuiltinDef describes a builtin tool definition: manifest bytes + an optional invoker factory.
//
// - Manifest is required for discovery (/tools).
// - NewInvoker is optional; some builtins may be "discoverable" but not executable yet.
type BuiltinDef struct {
	ID         types.ToolID
	Manifest   []byte
	NewInvoker func(cfg BuiltinConfig) ToolInvoker
}

var builtinDefs []BuiltinDef

// registerBuiltin registers a builtin tool definition into the tools package registry.
//
// This is intended to be called from init() functions in builtin tool files, e.g.:
//
//	func init() {
//	  registerBuiltin(BuiltinDef{...})
//	}
//
// The host then wires builtins into:
//   - /tools (for discovery) via tools.NewBuiltinManifestProvider + VirtualToolsResource
//   - ToolRunner (for execution) via BuiltinInvokerRegistry(...)
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
//
// This is typically used as the runner registry during early development, before
// adding external/custom tool execution.
func BuiltinInvokerRegistry(cfg BuiltinConfig) MapRegistry {
	out := make(MapRegistry)

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
