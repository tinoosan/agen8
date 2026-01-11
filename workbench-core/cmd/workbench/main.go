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

	// Create vfs
	fs := vfs.NewFS()

	// Create workspace
	workspace, err := resources.NewRunWorkspace(run.RunId)
	if err != nil {
		log.Fatalf("error creating workspace: %v", err)
	}

	fs.Mount(vfs.MountWorkspace, workspace)
	log.Printf("mounted workspace at %s", workspace.BaseDir)

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

}
