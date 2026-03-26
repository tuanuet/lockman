package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"lockman/lockkit/definitions"
	"lockman/lockkit/observe"
	"lockman/lockkit/registry"
	"lockman/lockkit/runtime"
	"lockman/lockkit/testkit"
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "example failed: %v\n", err)
		os.Exit(1)
	}
}

func run(out io.Writer) error {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderParentLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          10,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		return err
	}
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderItemLock",
		Kind:          definitions.KindChild,
		Resource:      "order_item",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          20,
		ParentRef:     "OrderParentLock",
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
	}); err != nil {
		return err
	}

	mgr, err := runtime.NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		return err
	}

	parentReq := definitions.SyncLockRequest{
		DefinitionID: "OrderParentLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			ServiceName: "example",
			InstanceID:  "local",
			HandlerName: "DependencyBoundary",
			OwnerID:     "example:local",
		},
	}

	err = mgr.ExecuteExclusive(context.Background(), parentReq, func(ctx context.Context, lease definitions.LeaseContext) error {
		if _, err := fmt.Fprintf(out, "parent: acquired %s\n", lease.ResourceKey); err != nil {
			return err
		}

		childReq := definitions.SyncLockRequest{
			DefinitionID: "OrderItemLock",
			KeyInput: map[string]string{
				"order_id": "123",
				"item_id":  "1",
			},
			Ownership: parentReq.Ownership,
		}

		return mgr.ExecuteExclusive(ctx, childReq, func(ctx context.Context, nested definitions.LeaseContext) error {
			if _, err := fmt.Fprintf(out, "child-like nested acquire: acquired %s\n", nested.ResourceKey); err != nil {
				return err
			}
			_, err := fmt.Fprintln(out, "note: nested child acquire succeeded because phase1 does not enforce parent-child dependency")
			return err
		})
	})
	if err != nil {
		return err
	}

	if err := mgr.Shutdown(context.Background()); err != nil {
		return err
	}

	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}
