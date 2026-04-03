//go:build lockman_examples

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/advanced/composite"
	lockredis "github.com/tuanuet/lockman/backend/redis"
)

func TestCompositeTransferOutput(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run failed: %v", err)
	}
	defer redisServer.Close()

	var out bytes.Buffer
	client := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer client.Close()

	if err := run(&out, client); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "transfer locked: account:acct-123,ledger:ledger-456") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestCompositeTransferBlocksStandaloneAccountRun(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run failed: %v", err)
	}
	defer redisServer.Close()

	redisClient := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()

	accountOnlyDef := lockman.DefineLock(
		"account",
		lockman.BindResourceID("account", func(in transferInput) string { return in.AccountID }),
	)
	ledgerOnlyDef := lockman.DefineLock(
		"ledger",
		lockman.BindResourceID("ledger", func(in transferInput) string { return in.LedgerID }),
	)
	transferOnlyDef := composite.DefineLock("transfer", accountOnlyDef, ledgerOnlyDef)
	transferOnly := composite.AttachRun("transfer.run.test", transferOnlyDef, lockman.TTL(5*time.Second))
	accountOnly := lockman.DefineRunOn("account.inspect", accountOnlyDef)

	reg := lockman.NewRegistry()
	if err := reg.Register(transferOnly, accountOnly); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	clientA, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "transfer-owner"}),
		lockman.WithBackend(lockredis.New(redisClient, "")),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer clientA.Shutdown(context.Background())

	clientB, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "account-owner"}),
		lockman.WithBackend(lockredis.New(redisClient, "")),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer clientB.Shutdown(context.Background())

	transferReq, err := transferOnly.With(transferInput{AccountID: "acct-123", LedgerID: "ledger-456"})
	if err != nil {
		t.Fatalf("transfer With returned error: %v", err)
	}
	accountReq, err := accountOnly.With(transferInput{AccountID: "acct-123"})
	if err != nil {
		t.Fatalf("account With returned error: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	compositeDone := make(chan error, 1)
	go func() {
		compositeDone <- clientA.Run(context.Background(), transferReq, func(context.Context, lockman.Lease) error {
			close(started)
			<-release
			return nil
		})
	}()

	<-started
	err = clientB.Run(context.Background(), accountReq, func(context.Context, lockman.Lease) error {
		return nil
	})
	if !errors.Is(err, lockman.ErrBusy) {
		close(release)
		t.Fatalf("expected ErrBusy while composite run holds account lock, got %v", err)
	}

	close(release)
	if err := <-compositeDone; err != nil {
		t.Fatalf("composite Run returned error: %v", err)
	}
}
