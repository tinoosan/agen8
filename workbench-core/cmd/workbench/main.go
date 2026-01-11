package main

import (
	"log"

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

	fs.Mount(vfs.MountWorkspace, workspace)
	log.Printf("mounted workspace at %s", workspace.BaseDir)
	fs.Mount(vfs.MountTrace, trace)
	log.Printf("mounted trace at %s", trace.BaseDir)
	
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

}
