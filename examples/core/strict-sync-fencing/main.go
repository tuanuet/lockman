package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/tuanuet/lockman/backend/memory"
	"github.com/tuanuet/lockman/lockkit/definitions"
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
		ID:                   "StrictOrderLock",
		Kind:                 definitions.KindParent,
		Resource:             "order",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionSync,
		LeaseTTL:             5 * time.Second,
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		return err
	}

	mgr, err := runtime.NewManager(reg, memory.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		return err
	}

	baseReq := definitions.SyncLockRequest{
		DefinitionID: "StrictOrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			ServiceName: "example",
			InstanceID:  "runtime",
			HandlerName: "Phase3aStrictRuntime",
			OwnerID:     "runtime-owner-a",
		},
	}

	var firstToken uint64
	if err := mgr.ExecuteExclusive(context.Background(), baseReq, func(ctx context.Context, lease definitions.LeaseContext) error {
		firstToken = lease.FencingToken
		if _, err := fmt.Fprintf(out, "strict runtime lock: %s\n", lease.ResourceKey); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "fencing token first: %d\n", lease.FencingToken)
		return err
	}); err != nil {
		return err
	}

	secondReq := baseReq
	secondReq.Ownership.OwnerID = "runtime-owner-b"
	var secondToken uint64
	if err := mgr.ExecuteExclusive(context.Background(), secondReq, func(ctx context.Context, lease definitions.LeaseContext) error {
		secondToken = lease.FencingToken
		_, err := fmt.Fprintf(out, "fencing token second: %d\n", lease.FencingToken)
		return err
	}); err != nil {
		return err
	}

	if secondToken <= firstToken {
		return fmt.Errorf("expected fencing token to increase across reacquire, first=%d second=%d", firstToken, secondToken)
	}

	if _, err := fmt.Fprintln(out, "teaching point: strict runtime exposes fencing tokens but still relies on one ttl window in phase3a"); err != nil {
		return err
	}

	if err := mgr.Shutdown(context.Background()); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}
