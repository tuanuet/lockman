package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/backend/memory"
	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/observe"
)

type orderInput struct {
	OrderID string
}

var orderDef = lockman.DefineLock(
	"order",
	lockman.BindResourceID("order", func(in orderInput) string { return in.OrderID }),
)

var approveOrder = lockman.DefineRunOn("order.approve", orderDef)

// processOrder omits Idempotent() to avoid needing an idempotency store.
// This is intentional for the zero-infra demo — Claim works fine without it.
var processOrder = lockman.DefineClaimOn(
	"order.process",
	orderDef,
)

func main() {
	ctx := context.Background()

	port := os.Getenv("DEMO_PORT")
	if port == "" {
		port = ":8080"
	}

	store := inspect.NewStore()

	dispatcher := observe.NewDispatcher(
		observe.WithExporter(observe.ExporterFunc(func(_ context.Context, e observe.Event) error {
			return nil
		})),
	)

	reg := lockman.NewRegistry()
	if err := reg.Register(approveOrder, processOrder); err != nil {
		panic(err)
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "inspect-demo"}),
		lockman.WithBackend(memory.NewMemoryDriver()),
		lockman.WithObservability(lockman.Observability{
			Dispatcher: dispatcher,
			Store:      store,
		}),
	)
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", inspect.NewHandler(store))

	srv := &http.Server{Addr: port, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}()

	fmt.Println("Inspect demo server running on " + port)
	fmt.Println("TUI:      go run ./cmd/inspect")
	fmt.Println("Snapshot: go run ./cmd/inspect snapshot")
	fmt.Println("Events:   go run ./cmd/inspect events --kind contention")
	fmt.Println("Health:   go run ./cmd/inspect health")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go runSyncTraffic(ctx, client)
	go runAsyncTraffic(ctx, client)
	go runContentionTraffic(ctx, client)

	<-stop

	fmt.Println("\nshutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	_ = client.Shutdown(shutdownCtx)
	fmt.Println("shutdown complete")
}

func runSyncTraffic(ctx context.Context, client *lockman.Client) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}

		orderID := fmt.Sprintf("ORD-%04d", rng.Intn(1000))
		req, err := approveOrder.With(orderInput{OrderID: orderID})
		if err != nil {
			continue
		}

		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_ = client.Run(reqCtx, req, func(_ context.Context, _ lockman.Lease) error {
			time.Sleep(time.Duration(rng.Intn(100)) * time.Millisecond)
			return nil
		})
		cancel()
	}
}

func runAsyncTraffic(ctx context.Context, client *lockman.Client) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	msgCounter := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}

		msgCounter++
		orderID := fmt.Sprintf("ORD-%04d", rng.Intn(1000))
		req, err := processOrder.With(orderInput{OrderID: orderID}, lockman.Delivery{
			MessageID:     fmt.Sprintf("msg-%d", msgCounter),
			ConsumerGroup: "demo-workers",
			Attempt:       1,
		})
		if err != nil {
			continue
		}

		reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_ = client.Claim(reqCtx, req, func(_ context.Context, _ lockman.Claim) error {
			time.Sleep(200 * time.Millisecond)
			return nil
		})
		cancel()
	}
}

func runContentionTraffic(ctx context.Context, client *lockman.Client) {
	const sharedOrderID = "ORD-CONTENTION"
	owners := []string{"owner-a", "owner-b"}

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		var wg sync.WaitGroup
		for _, owner := range owners {
			wg.Add(1)
			go func(owner string) {
				defer wg.Done()
				req, err := approveOrder.With(
					orderInput{OrderID: sharedOrderID},
					lockman.OwnerID(owner),
				)
				if err != nil {
					return
				}
				reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				_ = client.Run(reqCtx, req, func(_ context.Context, _ lockman.Lease) error {
					time.Sleep(1 * time.Second)
					return nil
				})
			}(owner)
		}
		wg.Wait()
	}
}
