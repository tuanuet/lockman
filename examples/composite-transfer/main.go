package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"lockman"
	"lockman/advanced/composite"
	lockredis "lockman/redis"
)

type transferInput struct {
	AccountID string
	LedgerID  string
}

var transferFunds = composite.DefineRunWithOptions(
	"transfer.run",
	[]lockman.UseCaseOption{
		lockman.TTL(5 * time.Second),
	},
	composite.DefineMember("account", lockman.BindResourceID("account", func(in transferInput) string { return in.AccountID })),
	composite.DefineMember("ledger", lockman.BindResourceID("ledger", func(in transferInput) string { return in.LedgerID })),
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
	if err := reg.Register(transferFunds); err != nil {
		return err
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "transfer-worker"}),
		lockman.WithBackend(lockredis.New(redisClient, "")),
	)
	if err != nil {
		return err
	}
	defer client.Shutdown(context.Background())

	req, err := transferFunds.With(transferInput{
		AccountID: "acct-123",
		LedgerID:  "ledger-456",
	})
	if err != nil {
		return err
	}

	if err := client.Run(context.Background(), req, func(_ context.Context, lease lockman.Lease) error {
		joined := strings.Join(lease.ResourceKeys, ",")
		if _, err := fmt.Fprintf(out, "transfer locked: %s\n", joined); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "lease ttl: %s\n", lease.LeaseTTL)
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
