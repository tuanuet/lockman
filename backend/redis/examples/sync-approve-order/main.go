package main

import (
	"context"
	"fmt"
	"io"
	"os"

	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman"
	lockredis "github.com/tuanuet/lockman/backend/redis"
)

type approveInput struct {
	OrderID string
}

var approveOrder = lockman.DefineRun[approveInput](
	"order.approve",
	lockman.BindResourceID("order", func(in approveInput) string { return in.OrderID }),
)

func main() {
	client, err := redisClientFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "example failed: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	if err := run(os.Stdout, client); err != nil {
		fmt.Fprintf(os.Stderr, "example failed: %v\n", err)
		os.Exit(1)
	}
}

func run(out io.Writer, redisClient goredis.UniversalClient) error {
	reg := lockman.NewRegistry()
	if err := reg.Register(approveOrder); err != nil {
		return err
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "orders-api"}),
		lockman.WithBackend(lockredis.New(redisClient, "")),
	)
	if err != nil {
		return err
	}
	defer client.Shutdown(context.Background())

	req, err := approveOrder.With(approveInput{OrderID: "123"})
	if err != nil {
		return err
	}

	if err := client.Run(context.Background(), req, func(_ context.Context, lease lockman.Lease) error {
		if _, err := fmt.Fprintf(out, "approved order: %s\n", "123"); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "lease key: %s\n", lease.ResourceKey)
		return err
	}); err != nil {
		return err
	}

	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}

func redisClientFromEnv() (*goredis.Client, error) {
	url := os.Getenv("LOCKMAN_REDIS_URL")
	if url == "" {
		url = "redis://127.0.0.1:6379/0"
	}
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return goredis.NewClient(opts), nil
}
