//go:build lockman_examples

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman"
	lockredis "github.com/tuanuet/lockman/backend/redis"
	idempotencyredis "github.com/tuanuet/lockman/idempotency/redis"
)

type processInput struct {
	OrderID string
}

var processOrder = lockman.DefineClaim[processInput](
	"order.process",
	lockman.BindResourceID("order", func(in processInput) string { return in.OrderID }),
	lockman.Idempotent(),
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
	if err := reg.Register(processOrder); err != nil {
		return err
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "orders-worker"}),
		lockman.WithBackend(lockredis.New(redisClient, "")),
		lockman.WithIdempotency(idempotencyredis.New(redisClient, "")),
	)
	if err != nil {
		return err
	}
	defer client.Shutdown(context.Background())

	req, err := processOrder.With(processInput{OrderID: "123"}, lockman.Delivery{
		MessageID:     "msg-1",
		ConsumerGroup: "orders",
		Attempt:       1,
	})
	if err != nil {
		return err
	}

	if err := client.Claim(context.Background(), req, func(_ context.Context, claim lockman.Claim) error {
		if _, err := fmt.Fprintf(out, "processed order: %s\n", "123"); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "idempotency key: %s\n", claim.IdempotencyKey)
		return err
	}); err != nil {
		return err
	}

	err = client.Claim(context.Background(), req, func(context.Context, lockman.Claim) error { return nil })
	if errors.Is(err, lockman.ErrDuplicate) {
		if _, writeErr := fmt.Fprintln(out, "duplicate ignored: msg-1"); writeErr != nil {
			return writeErr
		}
		err = nil
	}
	if err != nil {
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
