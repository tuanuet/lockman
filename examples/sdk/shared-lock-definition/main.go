//go:build lockman_examples

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

type contractInput struct {
	ContractID string
}

var contractDef = lockman.DefineLock(
	"contract",
	lockman.BindResourceID("contract", func(in contractInput) string { return in.ContractID }),
)

var contractImport = lockman.DefineRunOn("contract.import", contractDef)

var contractHold = lockman.DefineHoldOn("contract.manual_hold", contractDef)

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
	if err := reg.Register(contractImport, contractHold); err != nil {
		return err
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "contract-api"}),
		lockman.WithBackend(lockredis.New(redisClient, "")),
	)
	if err != nil {
		return err
	}
	defer client.Shutdown(context.Background())

	if _, err := fmt.Fprintf(out, "import use case: %s\n", contractImport.DefinitionID()); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "hold use case: %s\n", contractHold.DefinitionID()); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "shared definition ID: %s\n", contractDef.DefinitionID()); err != nil {
		return err
	}

	importReq, err := contractImport.With(contractInput{ContractID: "42"})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "import resource key: %s\n", importReq.ResourceKey()); err != nil {
		return err
	}

	holdReq, err := contractHold.With(contractInput{ContractID: "42"})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "hold resource key: %s\n", holdReq.ResourceKey()); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "teaching point: multiple use cases can share a single lock definition"); err != nil {
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
