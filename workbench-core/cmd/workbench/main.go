package main

import (
	"context"
	"fmt"
	"time"

	"github.com/tinoosan/workbench-core/internal/store"
)

func main() {
	// Create a new run
	run, err := store.CreateRun("TailEvents Demo", 200000)
	if err != nil {
		fmt.Printf("error creating run: %s\n", err)
		return
	}
	fmt.Printf("Created run: %s\n", run.RunId)

	// Get initial offset (0 for new file)
	_, offset, _ := store.ListEvents(run.RunId)

	// Start tailing events in a goroutine
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventCh, errCh := store.TailEvents(ctx, run.RunId, offset)

	// Append events in a separate goroutine
	go func() {
		for i := 1; i <= 5; i++ {
			time.Sleep(1 * time.Second)
			msg := fmt.Sprintf("Event #%d", i)
			err := store.AppendEvent(run.RunId, "demo-event", msg, map[string]string{"index": fmt.Sprintf("%d", i)})
			if err != nil {
				fmt.Printf("Error appending event: %s\n", err)
			}
			fmt.Printf("Appended: %s\n", msg)
		}
		// Give some time for the last event to be received, then cancel
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	// Receive and print events as they arrive
	fmt.Println("\n--- Tailing Events ---")
	var lastOffset int64
	for {
		select {
		case te, ok := <-eventCh:
			if !ok {
				fmt.Println("Event channel closed")
				return
			}
			lastOffset = te.NextOffset
			fmt.Printf("Received: [%s] %s - %s (NextOffset: %d)\n",
				te.Event.Type, te.Event.Message, te.Event.Timestamp.Format(time.RFC3339), te.NextOffset)
		case err := <-errCh:
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				return
			}
		case <-ctx.Done():
			fmt.Printf("Context done, exiting. Last offset: %d (save this for resume)\n", lastOffset)
			return
		}
	}
}
