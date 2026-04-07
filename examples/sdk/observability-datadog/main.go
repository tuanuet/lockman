//go:build lockman_examples

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentelemetry"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/tuanuet/lockman"
	lockredis "github.com/tuanuet/lockman/backend/redis"
	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/observe"
)

type approveInput struct {
	OrderID string
}

var orderDef = lockman.DefineLock(
	"order",
	lockman.BindResourceID("order", func(in approveInput) string { return in.OrderID }),
)

var approveOrder = lockman.DefineRunOn("order.approve", orderDef)

func main() {
	ctx := context.Background()

	// Start Datadog tracer — this auto-instruments HTTP, DB, etc.
	tracerOpts := []tracer.StartOption{
		tracer.WithService("lockman-example"),
		tracer.WithEnv("development"),
	}
	if agentAddr := os.Getenv("DD_AGENT_HOST"); agentAddr != "" {
		tracerOpts = append(tracerOpts, tracer.WithAgentAddr(agentAddr))
	}
	tracer.Start(tracerOpts...)
	defer tracer.Stop()

	// Wrap Datadog tracer as an OpenTelemetry TracerProvider.
	// The OTelSink accepts this directly — no Datadog-specific code in lockman.
	ddTracerProvider := opentelemetry.NewTracerProvider()

	// Create an OTel sink that emits lockman events as Datadog spans.
	otelSink := observe.NewOTelSink(observe.OTelConfig{
		TracerProvider: ddTracerProvider,
	})

	// Create a dispatcher that routes events to the OTel sink.
	dispatcher := observe.NewDispatcher(
		observe.WithSink(otelSink),
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
	redisAddr := "localhost:6379"
	if url := os.Getenv("LOCKMAN_REDIS_URL"); url != "" {
		redisAddr = url
	}
	redisClient := goredis.NewClient(&goredis.Options{Addr: redisAddr})

	// Create client with observability wired to Datadog.
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

	// Run a use case — each lifecycle event becomes a Datadog span.
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

	// Wait briefly for async events to flush to Datadog.
	time.Sleep(100 * time.Millisecond)

	// Print recent events.
	events := store.RecentEvents(10)
	fmt.Printf("recent events: %d\n", len(events))
}
