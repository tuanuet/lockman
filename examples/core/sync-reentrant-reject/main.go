package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/backend/memory"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/observe"
	"github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/lockkit/runtime"
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
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		return err
	}

	mgr, err := runtime.NewManager(reg, memory.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		return err
	}

	req := definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			ServiceName: "example",
			InstanceID:  "local",
			HandlerName: "ReentrantBoundary",
			OwnerID:     "example:local",
		},
	}

	err = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		if _, err := fmt.Fprintf(out, "outer: acquired %s\n", lease.ResourceKey); err != nil {
			return err
		}

		nestedErr := mgr.ExecuteExclusive(ctx, req, func(ctx context.Context, nested definitions.LeaseContext) error {
			_, err := fmt.Fprintf(out, "nested same lock: acquired %s\n", nested.ResourceKey)
			return err
		})
		if errors.Is(nestedErr, lockerrors.ErrReentrantAcquire) {
			_, err := fmt.Fprintln(out, "nested same lock: reentrant acquire")
			return err
		}
		return nestedErr
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
