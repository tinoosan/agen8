package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

type invokerFunc func(ctx context.Context, req types.ToolRequest) (tools.ToolCallResult, error)

func (f invokerFunc) Invoke(ctx context.Context, req types.ToolRequest) (tools.ToolCallResult, error) {
	return f(ctx, req)
}

func main() {
	log.Printf("== Workbench demo starting ==")

	run, err := store.CreateRun("test goal", 100)
	if err != nil {
		log.Fatalf("error creating run: %v", err)
	}
	log.Printf("runId=%s", run.RunId)

	data := map[string]string{
		"name":  "Alice",
		"email": "alice@example.com",
	}

	log.Printf("== Trace setup (events mirrored into /trace) ==")

	err = store.AppendEvent(run.RunId, "run.started", "Run started", data)
	if err != nil {
		log.Fatalf("error appending event: %v", err)
	}
	err = store.AppendEvent(run.RunId, "step.started", "Step 1 started", data)
	if err != nil {
		log.Fatalf("error appending event: %v", err)
	}
	err = store.AppendEvent(run.RunId, "step.progress", "Step 1 halfway", data)
	if err != nil {
		log.Fatalf("error appending event: %v", err)
	}
	err = store.AppendEvent(run.RunId, "step.completed", "Step 1 done", data)
	if err != nil {
		log.Fatalf("error appending event: %v", err)
	}
	err = store.AppendEvent(run.RunId, "run.completed", "Run finished", data)
	if err != nil {
		log.Fatalf("error appending event: %v", err)
	}

	trace, err := resources.NewTraceResource(run.RunId)
	if err != nil {
		log.Fatalf("error creating trace: %v", err)
	}

	fs := vfs.NewFS()

	workspace, err := resources.NewRunWorkspace(run.RunId)
	if err != nil {
		log.Fatalf("error creating workspace: %v", err)
	}

	toolsResource, err := resources.NewToolsResource()
	if err != nil {
		log.Fatalf("error creating tools: %v", err)
	}
	if err := toolsResource.RegisterBuiltin(
		"github.com.acme.stock",
		[]byte(`{"id":"github.com.acme.stock","version":"0.1.0","kind":"builtin","displayName":"Acme Stock","description":"Example builtin tool","actions":[{"id":"quote.latest","displayName":"Latest Quote","description":"Fetch latest quote for a symbol","inputSchema":{"type":"object","properties":{"symbol":{"type":"string"}},"required":["symbol"]},"outputSchema":{"type":"object","properties":{"symbol":{"type":"string"},"price":{"type":"number"}},"required":["symbol","price"]}}]}`),
	); err != nil {
		log.Fatalf("error registering builtin tool manifest: %v", err)
	}
	if err := toolsResource.RegisterBuiltin(
		"github.com.dupe.tool",
		[]byte(`{"id":"github.com.dupe.tool","version":"0.1.0","kind":"builtin","displayName":"Dupe Tool (builtin)","description":"Builtin should override disk","actions":[{"id":"dupe.noop","displayName":"No-op","description":"Returns an empty object","inputSchema":{"type":"object"},"outputSchema":{"type":"object"}}]}`),
	); err != nil {
		log.Fatalf("error registering builtin tool manifest: %v", err)
	}

	log.Printf("== Tools setup (builtins + disk) ==")

	// Disk tools under data/tools/<toolId>/manifest.json
	if err := os.MkdirAll(filepath.Join(toolsResource.BaseDir, "github.com.other.stock"), 0755); err != nil {
		log.Fatalf("error creating disk tool directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(toolsResource.BaseDir, "github.com.other.stock", "manifest.json"),
		[]byte(`{"id":"github.com.other.stock","version":"0.1.0","kind":"custom","displayName":"Other Stock","description":"Example disk tool","actions":[{"id":"quote.latest","displayName":"Latest Quote","description":"Fetch latest quote for a symbol","inputSchema":{"type":"object","properties":{"symbol":{"type":"string"}},"required":["symbol"]},"outputSchema":{"type":"object","properties":{"symbol":{"type":"string"},"price":{"type":"number"}},"required":["symbol","price"]}}]}`),
		0644,
	); err != nil {
		log.Fatalf("error writing disk tool manifest: %v", err)
	}
	// Collision on disk: should be ignored on read due to builtin override
	if err := os.MkdirAll(filepath.Join(toolsResource.BaseDir, "github.com.dupe.tool"), 0755); err != nil {
		log.Fatalf("error creating collision tool directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(toolsResource.BaseDir, "github.com.dupe.tool", "manifest.json"),
		[]byte(`{"id":"github.com.dupe.tool","version":"0.1.0","kind":"custom","displayName":"Dupe Tool (disk)","description":"Should not be returned","actions":[{"id":"dupe.noop","displayName":"No-op","description":"Returns an empty object","inputSchema":{"type":"object"},"outputSchema":{"type":"object"}}]}`),
		0644,
	); err != nil {
		log.Fatalf("error writing collision tool manifest: %v", err)
	}

	results, err := resources.NewRunResults(run.RunId)
	if err != nil {
		log.Fatalf("error creating results: %v", err)
	}

	fs.Mount(vfs.MountWorkspace, workspace)
	log.Printf("mounted /workspace => %s", workspace.BaseDir)
	fs.Mount(vfs.MountResults, results)
	log.Printf("mounted /results => %s", results.BaseDir)
	fs.Mount(vfs.MountTrace, trace)
	log.Printf("mounted /trace => %s", trace.BaseDir)
	fs.Mount(vfs.MountTools, toolsResource)
	log.Printf("mounted /tools => %s", toolsResource.BaseDir)

	log.Printf("== VFS mounts ==")
	mounts, err := fs.List("/")
	if err != nil {
		log.Fatalf("error listing mounts: %v", err)
	}
	for _, e := range mounts {
		log.Printf("mount: %s", e.Path)
	}

	log.Printf("== Workspace demo (read/write/append/list) ==")

	if err := fs.Write("/workspace/notes.md", []byte("hello world")); err != nil {
		log.Fatalf("error writing to workspace: %v", err)
	}

	b, err := fs.Read("/workspace/notes.md")
	if err != nil {
		log.Fatalf("error reading from workspace: %v", err)
	}
	log.Printf("read /workspace/notes.md => %q", string(b))

	if err := fs.Append("/workspace/notes.md", []byte("\n\nHello again!")); err != nil {
		log.Fatalf("error appending to workspace: %v", err)
	}

	entries, err := fs.List("/workspace")
	if err != nil {
		log.Fatalf("error listing workspace: %v", err)
	}
	for _, e := range entries {
		log.Printf("workspace entry: %s", e.Path)
	}

	log.Printf("== Trace demo (latest + since) ==")

	entries, err = fs.List("/trace")
	if err != nil {
		log.Fatalf("error listing trace: %v", err)
	}
	for _, e := range entries {
		log.Printf("trace capability: %s", e.Path)
	}

	traceLatest, err := fs.Read("/trace/events.latest/3")
	if err != nil {
		log.Fatalf("error reading trace events.latest: %v", err)
	}
	log.Printf("read /trace/events.latest/3 =>\n%s", string(traceLatest))

	// Offset-based incremental read (UI-style):
	// 1) ListEvents gives nextOffset (byte offset)
	_, nextOffset, err := store.ListEvents(run.RunId)
	if err != nil {
		log.Fatalf("error listing events for offset: %v", err)
	}
	// 2) Later, new events arrive...
	err = store.AppendEvent(run.RunId, "agent.thought", "Thinking...", nil)
	if err != nil {
		log.Fatalf("error appending event: %v", err)
	}
	err = store.AppendEvent(run.RunId, "agent.action", "Did something", nil)
	if err != nil {
		log.Fatalf("error appending event: %v", err)
	}
	// 3) Fetch new bytes from trace using that offset
	traceSince, err := fs.Read("/trace/events.since/" + strconv.FormatInt(nextOffset, 10))
	if err != nil {
		log.Fatalf("error reading trace events.since: %v", err)
	}
	log.Printf("read /trace/events.since/%d =>\n%s", nextOffset, string(traceSince))

	log.Printf("== Tools demo (discovery + read manifest bytes) ==")

	entries, err = fs.List("/tools")
	if err != nil {
		log.Fatalf("error listing tools: %v", err)
	}
	for _, e := range entries {
		log.Printf("tool: %s", e.Path)
	}

	acmeManifest, err := fs.Read("/tools/github.com.acme.stock")
	if err != nil {
		log.Fatalf("error reading builtin tool manifest: %v", err)
	}
	log.Printf("builtin manifest github.com.acme.stock:\n%s", string(acmeManifest))

	otherManifest, err := fs.Read("/tools/github.com.other.stock/manifest.json")
	if err != nil {
		log.Fatalf("error reading disk tool manifest: %v", err)
	}
	log.Printf("disk manifest github.com.other.stock:\n%s", string(otherManifest))

	dupeManifest, err := fs.Read("/tools/github.com.dupe.tool")
	if err != nil {
		log.Fatalf("error reading dupe tool manifest: %v", err)
	}
	log.Printf("dupe manifest (builtin overrides disk) github.com.dupe.tool:\n%s", string(dupeManifest))

	log.Printf("== Tool runner demo (writes /results/<callId>/...) ==")

	runner := tools.Runner{
		FS: fs,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("github.com.acme.stock"): invokerFunc(func(ctx context.Context, req types.ToolRequest) (tools.ToolCallResult, error) {
				return tools.ToolCallResult{
					Output: json.RawMessage(`{"ok":true,"price":123.45}`),
					Artifacts: []tools.ToolArtifactWrite{
						{Path: "quote.json", Bytes: []byte(`{"symbol":"AAPL","price":123.45}`), MediaType: "application/json"},
						{Path: "notes.md", Bytes: []byte("# Quote\nAAPL = 123.45\n"), MediaType: "text/markdown"},
					},
				}, nil
			}),
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("github.com.acme.stock"), "quote.latest", json.RawMessage(`{"symbol":"AAPL"}`), 0)
	if err != nil {
		log.Fatalf("runner.Run failed: %v", err)
	}
	pretty, _ := json.MarshalIndent(resp, "", "  ")
	log.Printf("runner response =>\n%s", string(pretty))

	responsePath := "/results/" + resp.CallID + "/response.json"
	responseBytes, err := fs.Read(responsePath)
	if err != nil {
		log.Fatalf("read persisted response.json failed: %v", err)
	}
	log.Printf("read %s =>\n%s", responsePath, string(responseBytes))

	for _, a := range resp.Artifacts {
		p := "/results/" + resp.CallID + "/" + a.Path
		b, err := fs.Read(p)
		if err != nil {
			log.Fatalf("read artifact %s failed: %v", p, err)
		}
		log.Printf("read %s (%s) => %d bytes", p, a.MediaType, len(b))
	}

	log.Printf("== Workbench demo complete ==")
}
