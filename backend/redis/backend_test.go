package redis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"lockman"
)

func TestNewBackendCanBeUsedWithRootClientOption(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run failed: %v", err)
	}
	defer redisServer.Close()

	reg := lockman.NewRegistry()
	approve := lockman.DefineRun[string](
		"order.approve",
		lockman.BindResourceID("order", func(v string) string { return v }),
	)
	if err := reg.Register(approve); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	clientConn := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer func() {
		_ = clientConn.Close()
	}()

	backend := New(clientConn, "")

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "owner-1"}),
		lockman.WithBackend(backend),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}

	req, err := approve.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	if err := client.Run(context.Background(), req, func(context.Context, lockman.Lease) error { return nil }); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}
