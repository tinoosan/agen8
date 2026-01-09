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

	data := map[string]string{
		"some_key": "some_value",
	}

	err = store.AppendEvent(run.RunId, "run-created", "some generic message", data)
	if err != nil {
		fmt.Println(err)
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

	err = store.AppendEvent(stoppedRun.RunId, "run-stopped", "some generic message", data)
	if err != nil {
		fmt.Println(err)
		return
	}

	events, err := store.ListEvents(stoppedRun.RunId)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(events)

}
