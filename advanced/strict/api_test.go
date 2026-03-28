package strict

import (
	"context"
	"testing"

	"lockman"
	"lockman/lockkit/testkit"
)

func TestStrictPackageExposesPublicRunUseCaseAuthoring(t *testing.T) {
	reg := lockman.NewRegistry()
	approve := DefineRun[string](
		"order.strict-approve",
		lockman.BindResourceID("order", func(v string) string { return v }),
	)
	if err := reg.Register(approve); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "owner-a"}),
		lockman.WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	reqA, err := approve.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	var firstToken uint64
	if err := client.Run(context.Background(), reqA, func(_ context.Context, lease lockman.Lease) error {
		firstToken = lease.FencingToken
		return nil
	}); err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}
	if firstToken == 0 {
		t.Fatal("expected fencing token from strict run")
	}

	reqB, err := approve.With("123", lockman.OwnerID("owner-b"))
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	var secondToken uint64
	if err := client.Run(context.Background(), reqB, func(_ context.Context, lease lockman.Lease) error {
		secondToken = lease.FencingToken
		return nil
	}); err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}
	if secondToken <= firstToken {
		t.Fatalf("expected fencing token to increase, first=%d second=%d", firstToken, secondToken)
	}
}
