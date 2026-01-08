package main

import (
	"fmt"

	"github.com/tinoosan/workbench-core/internal/store"
)

func main() {
	_, err := store.CreateRun("some generic goal", 200000)
	if err != nil {
		fmt.Printf("error: %s\n", err)
	}
}
