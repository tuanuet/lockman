//go:build lockman_examples

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/tuanuet/lockman/observe"
)

func main() {
	ctx := context.Background()

	// Create a dispatcher for async event export.
	dispatcher := observe.NewDispatcher(
		observe.WithExporter(observe.ExporterFunc(func(_ context.Context, e observe.Event) error {
			fmt.Printf("event: %s\n", e.Kind)
			return nil
		})),
	)
	defer func() { _ = dispatcher.Shutdown(ctx) }()

	fmt.Println("observability runtime example ready")
	fmt.Println("dispatcher created and configured")

	// Publish a test event.
	dispatcher.Publish(observe.Event{
		Kind:       observe.EventAcquireStarted,
		OwnerID:    "example",
		ResourceID: "demo:1",
	})

	// Wait briefly for async events.
	time.Sleep(100 * time.Millisecond)
	fmt.Println("done")
}
