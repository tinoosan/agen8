package main

import (
	"fmt"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/types"
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

	fmt.Printf("loaded run: %v\n", loadedRun)

	stoppedRun, err := store.StopRun(loadedRun.RunId, types.StatusFailed, "some generic error")
	if err != nil {
		fmt.Printf("error: %s\n", err)
		return
	}

	fmt.Printf("stopped run: %+v\n", stoppedRun)

}
