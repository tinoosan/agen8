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
	log.Printf("mounted workspace at %s", workspace.Mount)

	fs.Write("/workspace/notes.md", []byte("hello world"))
	log.Printf("wrote /workspace/notes.md")

	entries, err := fs.List("/")
	if err != nil {
		log.Fatalf("error listing /: %v", err)
	}
	for _, e := range entries {
		log.Printf("entry: %+v", e)
	}
	

}
