package main

import (
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func main() {

	// Create a run
	run, err := store.CreateRun("test goal", 100)
	if err != nil {
		log.Fatalf("error creating run: %v", err)
	}

	data := map[string]string{
		"name":  "Alice",
		"email": "alice@example.com",
	}

	// Create event log
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

	// create trace
	trace, err := resources.NewTraceResource(run.RunId)
	if err != nil {
		log.Fatalf("error creating trace: %v", err)
	}

	// Create vfs
	fs := vfs.NewFS()

	// Create workspace
	workspace, err := resources.NewRunWorkspace(run.RunId)
	if err != nil {
		log.Fatalf("error creating workspace: %v", err)
	}

	// Create tools
	tools, err := resources.NewToolsResource()
	if err != nil {
		log.Fatalf("error creating tools: %v", err)
	}
	tools.BuiltinRegistry["github.com.acme.stock"] = resources.BuiltinTool{
		Manifest: []byte(`{"id":"github.com.acme.stock","version":"0.1.0","kind":"builtin","displayName":"Acme Stock","description":"Example builtin tool","actions":[]}`),
	}
	tools.BuiltinRegistry["github.com.dupe.tool"] = resources.BuiltinTool{
		Manifest: []byte(`{"id":"github.com.dupe.tool","version":"0.1.0","kind":"builtin","displayName":"Dupe Tool (builtin)","description":"Builtin should override disk","actions":[]}`),
	}

	// Add custom-on-disk tools (data/tools/<toolId>/manifest.json)
	if err := os.MkdirAll(filepath.Join(tools.BaseDir, "github.com.other.stock"), 0755); err != nil {
		log.Fatalf("error creating disk tool directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(tools.BaseDir, "github.com.other.stock", "manifest.json"),
		[]byte(`{"id":"github.com.other.stock","version":"0.1.0","kind":"custom","displayName":"Other Stock","description":"Example disk tool","actions":[]}`),
		0644,
	); err != nil {
		log.Fatalf("error writing disk tool manifest: %v", err)
	}
	// Collision on disk: should be ignored on read due to builtin override
	if err := os.MkdirAll(filepath.Join(tools.BaseDir, "github.com.dupe.tool"), 0755); err != nil {
		log.Fatalf("error creating collision tool directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(tools.BaseDir, "github.com.dupe.tool", "manifest.json"),
		[]byte(`{"id":"github.com.dupe.tool","version":"0.1.0","kind":"custom","displayName":"Dupe Tool (disk)","description":"Should not be returned","actions":[]}`),
		0644,
	); err != nil {
		log.Fatalf("error writing collision tool manifest: %v", err)
	}

	// Create results
	results, err := resources.NewRunResults(run.RunId)
	if err != nil {
		log.Fatalf("error creating results: %v", err)
	}

	fs.Mount(vfs.MountWorkspace, workspace)
	log.Printf("mounted workspace at %s", workspace.BaseDir)
	fs.Mount(vfs.MountResults, results)
	log.Printf("mounted results at %s", results.BaseDir)
	fs.Mount(vfs.MountTrace, trace)
	log.Printf("mounted trace at %s", trace.BaseDir)
	fs.Mount(vfs.MountTools, tools)
	log.Printf("mounted tools at %s", tools.BaseDir)

	// Write to workspace
	if err := fs.Write("/workspace/notes.md", []byte("hello world")); err != nil {
		log.Fatalf("error writing to workspace: %v", err)
	}

	// Read from workspace
	b, err := fs.Read("/workspace/notes.md")
	if err != nil {
		log.Fatalf("error reading from workspace: %v", err)
	}
	log.Printf("read from workspace: %s", string(b))

	// Append to workspace
	if err := fs.Append("/workspace/notes.md", []byte("\n\nHello again!")); err != nil {
		log.Fatalf("error appending to workspace: %v", err)
	}

	// List workspace
	entries, err := fs.List("/workspace")
	if err != nil {
		log.Fatalf("error listing workspace: %v", err)
	}
	for _, e := range entries {
		log.Printf("entry: %+v", e)
	}

	// List root
	entries, err = fs.List("/")
	if err != nil {
		log.Fatalf("error listing root: %v", err)
	}
	for _, e := range entries {
		log.Printf("entry: %+v", e)
	}

	// List trace
	entries, err = fs.List("/trace")
	if err != nil {
		log.Fatalf("error listing trace: %v", err)
	}
	for _, e := range entries {
		log.Printf("entry: %+v", e)
	}

	// Read trace events and print the last 3 lines
	traceLatest, err := fs.Read("/trace/events.latest/3")
	if err != nil {
		log.Fatalf("error reading trace events.latest: %v", err)
	}
	log.Printf("trace latest 3:\n%s", string(traceLatest))

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
	log.Printf("trace since offset %d:\n%s", nextOffset, string(traceSince))

	// List tools and read manifests
	entries, err = fs.List("/tools")
	if err != nil {
		log.Fatalf("error listing tools: %v", err)
	}
	for _, e := range entries {
		log.Printf("tool: %+v", e)
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

}
