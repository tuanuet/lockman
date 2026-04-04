//go:build lockman_examples

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman"
	lockredis "github.com/tuanuet/lockman/backend/redis"
)

type batchOrderInput struct {
	OrderID string
}

var orderDef = lockman.DefineLock(
	"order",
	lockman.BindResourceID("order", func(in batchOrderInput) string { return in.OrderID }),
)

var batchProcess = lockman.DefineRunOn("batch_process", orderDef, lockman.TTL(5*time.Second))

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
	if err := reg.Register(batchProcess); err != nil {
		return err
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "batch-worker"}),
		lockman.WithBackend(lockredis.New(redisClient, "")),
	)
	if err != nil {
		return err
	}
	defer client.Shutdown(context.Background())

	keys := []string{"order:1", "order:2", "order:3"}

	if err := client.RunMultiple(context.Background(), batchProcess, func(_ context.Context, lease lockman.Lease) error {
		joined := strings.Join(lease.ResourceKeys, ",")
		if _, err := fmt.Fprintf(out, "batch locked: %s\n", joined); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "lease ttl: %s\n", lease.LeaseTTL)
		return err
	}, batchOrderInput{}, keys); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "shutdown: ok"); err != nil {
		return err
	}
	return nil
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
