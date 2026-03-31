package lockman

import (
	"context"
	"fmt"
	"testing"

	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/lockkit/testkit"
	"github.com/tuanuet/lockman/observe"
)

func TestDebugBridge(t *testing.T) {
	d := observe.NewDispatcher()
	defer func() { _ = d.Shutdown(context.Background()) }()
	store := inspect.NewStore()

	reg := NewRegistry()
	uc := testRunUseCase("order.approve")
	mustRegisterUseCases(t, reg, uc)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
		WithObservability(Observability{Dispatcher: d, Store: store}),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	fmt.Printf("bridge nil? %v\n", client.bridge == nil)
	fmt.Printf("runtime nil? %v\n", client.runtime == nil)

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	err = client.Run(context.Background(), req, func(context.Context, Lease) error { return nil })
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	snap := store.Snapshot()
	fmt.Printf("RuntimeLocks: %d\n", len(snap.RuntimeLocks))
	fmt.Printf("WorkerClaims: %d\n", len(snap.WorkerClaims))
	events := store.RecentEvents(20)
	fmt.Printf("Events: %d\n", len(events))
	for i, e := range events {
		fmt.Printf("  event[%d]: kind=%v defID=%s resID=%s ownerID=%s\n", i, e.Kind, e.DefinitionID, e.ResourceID, e.OwnerID)
	}
}
