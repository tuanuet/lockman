package redis

import (
	"testing"

	goredis "github.com/redis/go-redis/v9"

	"lockman"
)

func TestNewBackendCanBeUsedWithRootClientOption(t *testing.T) {
	reg := lockman.NewRegistry()
	approve := lockman.DefineRun[string](
		"order.approve",
		lockman.BindResourceID("order", func(v string) string { return v }),
	)
	if err := reg.Register(approve); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	backend := New(goredis.NewClient(&goredis.Options{
		Addr: "127.0.0.1:6379",
	}), "")

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
}
