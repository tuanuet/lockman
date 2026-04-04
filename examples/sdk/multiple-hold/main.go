//go:build lockman_examples

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman"
	lockredis "github.com/tuanuet/lockman/backend/redis"
)

type reserveInput struct {
	SlotID string
}

var slotDef = lockman.DefineLock(
	"slot",
	lockman.BindResourceID("slot", func(in reserveInput) string { return in.SlotID }),
)

var reserveSlots = lockman.DefineHoldOn("reserve_slots", slotDef, lockman.TTL(30*time.Second))

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
	if err := reg.Register(reserveSlots); err != nil {
		return err
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "warehouse-api"}),
		lockman.WithBackend(lockredis.New(redisClient, "")),
	)
	if err != nil {
		return err
	}
	defer client.Shutdown(context.Background())

	keys := []string{"slot:A", "slot:B", "slot:C"}

	handle, err := client.HoldMultiple(context.Background(), reserveSlots, reserveInput{}, keys)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(out, "hold keys: %s\n", keys); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "hold token: %s\n", handle.Token()); err != nil {
		return err
	}

	if err := client.Forfeit(context.Background(), reserveSlots.ForfeitWith(handle.Token())); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "forfeit: ok"); err != nil {
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
