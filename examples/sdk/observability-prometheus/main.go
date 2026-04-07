//go:build lockman_examples

package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman"
	lockredis "github.com/tuanuet/lockman/backend/redis"
	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/observe"
	"github.com/tuanuet/lockman/observe/prometheus"
)

type OrderInput struct {
	OrderID string
}

func main() {
	ctx := context.Background()

	orderDef := lockman.DefineLock(
		"order",
		lockman.BindResourceID("order", func(in OrderInput) string { return in.OrderID }),
	)
	approveOrder := lockman.DefineRunOn("order.approve", orderDef)

	reg := lockman.NewRegistry()
	if err := reg.Register(approveOrder); err != nil {
		panic(err)
	}

	promSink := prometheus.NewPrometheusSink(prometheus.PrometheusConfig{
		Namespace: "lockman",
	})

	dispatcher := observe.NewDispatcher(
		observe.WithSink(promSink),
	)
	defer func() { _ = dispatcher.Shutdown(ctx) }()

	store := inspect.NewStore()

	redisAddr := "localhost:6379"
	redisClient := goredis.NewClient(&goredis.Options{Addr: redisAddr})

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

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/locks/", inspect.NewHandler(store))
	go func() {
		fmt.Println("metrics available at http://localhost:9090/metrics")
		http.ListenAndServe(":9090", mux)
	}()

	req, err := approveOrder.With(OrderInput{OrderID: "123"})
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

	select {}
}
