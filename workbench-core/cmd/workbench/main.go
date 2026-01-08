package main

import (
	"fmt"

	"github.com/tinoosan/workbench-core/internal/store"
)

func main() {
	run, err := store.CreateRun("some generic goal", 200000)
	if err != nil {
		fmt.Printf("error: %s\n", err)
		return
	}

	loadedRun, err := store.LoadRun(run.RunId)
	if err != nil {
		fmt.Printf("error: %s\n", err)
		return
	}

	fmt.Printf("loaded run: %v", loadedRun)
}
