//go:build lockman_examples

package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman"
	lockredis "github.com/tuanuet/lockman/backend/redis"
	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/observe"
)

type approveInput struct {
	OrderID string
}

var approveOrder = lockman.DefineRun[approveInput](
	"order.approve",
	lockman.BindResourceID("order", func(in approveInput) string { return in.OrderID }),
)

func main() {
	ctx := context.Background()

	// Create a dispatcher for async event export.
	dispatcher := observe.NewDispatcher(
		observe.WithExporter(observe.ExporterFunc(func(_ context.Context, e observe.Event) error {
			fmt.Printf("event: %s (%s)\n", e.Kind, e.ResourceID)
			return nil
		})),
	)
	defer func() { _ = dispatcher.Shutdown(ctx) }()

	// Create an inspect store for process-local state.
	store := inspect.NewStore()

	// Register use case.
	reg := lockman.NewRegistry()
	if err := reg.Register(approveOrder); err != nil {
		panic(err)
	}

	// Create backend.
	redisClient := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})

	// Create client with observability.
	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "example-api"}),
		lockman.WithBackend(lockredis.New(redisClient, "")),
		lockman.WithObservability(lockman.Observability{
			Dispatcher: dispatcher,
			Store:      store,
		}),
	)
	if err != nil {
		panic(err)
	}
	defer client.Shutdown(ctx)

	// Mount inspection HTTP handlers.
	mux := http.NewServeMux()
	mux.Handle("/locks/", inspect.NewHandler(store))
	fmt.Println("inspect endpoint: /locks/inspect")

	// Run a simple use case.
	req, err := approveOrder.With(approveInput{OrderID: "123"})
	if err != nil {
		panic(err)
	}
	err = client.Run(ctx, req, func(_ context.Context, _ lockman.Lease) error {
		fmt.Println("executing order approval")
		return nil
	})
	if err != nil {
		panic(err)
	}

	// Wait briefly for async events to flush.
	time.Sleep(100 * time.Millisecond)

	// Print recent events.
	events := store.RecentEvents(10)
	fmt.Printf("recent events: %d\n", len(events))
}
