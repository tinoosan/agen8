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

	// List via VFS path (instead of workspace.List(""))
	mount, r, subpath, err := fs.Resolve("/workspace")
	if err != nil {
		log.Fatalf("resolve list path: %v", err)
	}
	entries, err := r.List(subpath)
	if err != nil {
		log.Fatalf("list %s: %v", mount, err)
	}

	for _, e := range entries {
		log.Printf("entry: %+v", e)
	}

	// Write via VFS path
	mount, r, subpath, err = fs.Resolve("/workspace/hello.txt")
	if err != nil {
		log.Fatalf("resolve write path: %v", err)
	}
	if err := r.Write(subpath, []byte("hello world\n")); err != nil {
		log.Fatalf("write %s: %v", mount, err)
	}

	// Read via VFS path
	mount, r, subpath, err = fs.Resolve("/workspace/hello.txt")
	if err != nil {
		log.Fatalf("resolve read path: %v", err)
	}
	b, err := r.Read(subpath)
	if err != nil {
		log.Fatalf("read %s: %v", mount, err)
	}
	log.Printf("read: %q", string(b))

}
