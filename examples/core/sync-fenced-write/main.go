//go:build lockman_examples

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/advanced/strict"
	lockredis "github.com/tuanuet/lockman/backend/redis"
)

type writeInput struct {
	OrderID string
}

var strictWriteDef = lockman.DefineLock(
	"order.strict-write",
	lockman.BindResourceID("order", func(in writeInput) string { return in.OrderID }),
)

var strictWrite = strict.DefineRunOn("order.strict-write", strictWriteDef)

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
	if err := reg.Register(strictWrite); err != nil {
		return err
	}

	clientA, err := newLockmanClient(reg, redisClient, "writer-a")
	if err != nil {
		return err
	}
	defer clientA.Shutdown(context.Background())

	clientB, err := newLockmanClient(reg, redisClient, "writer-b")
	if err != nil {
		return err
	}
	defer clientB.Shutdown(context.Background())

	firstReq, err := strictWrite.With(writeInput{OrderID: "123"})
	if err != nil {
		return err
	}

	var firstToken uint64
	if err := clientA.Run(context.Background(), firstReq, func(_ context.Context, lease lockman.Lease) error {
		firstToken = lease.FencingToken
		if _, err := fmt.Fprintf(out, "fencing token first: %d\n", lease.FencingToken); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "strict write key: %s\n", lease.ResourceKey)
		return err
	}); err != nil {
		return err
	}

	secondReq, err := strictWrite.With(writeInput{OrderID: "123"})
	if err != nil {
		return err
	}

	var secondToken uint64
	if err := clientB.Run(context.Background(), secondReq, func(_ context.Context, lease lockman.Lease) error {
		secondToken = lease.FencingToken
		_, err := fmt.Fprintf(out, "fencing token second: %d\n", lease.FencingToken)
		return err
	}); err != nil {
		return err
	}

	if secondToken <= firstToken {
		return fmt.Errorf("expected fencing token to increase, first=%d second=%d", firstToken, secondToken)
	}

	if _, err := fmt.Fprintln(out, "fencing token increased"); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}

func newLockmanClient(reg *lockman.Registry, redisClient goredis.UniversalClient, ownerID string) (*lockman.Client, error) {
	return lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: ownerID}),
		lockman.WithBackend(lockredis.New(redisClient, "")),
	)
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
