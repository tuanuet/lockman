package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"lockman/backend"
	"lockman/lockkit/definitions"
	lockerrors "lockman/lockkit/errors"
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

// This example uses the driver directly so TTL expiry is visible without the
// runtime's callback-scoped auto-release masking it.
func run(out io.Writer) error {
	const leaseTTL = 40 * time.Millisecond

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      leaseTTL,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		return err
	}

	driver := testkit.NewMemoryDriver()
	mgr, err := runtime.NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		return err
	}

	resourceKey := "order:123"
	leaseA, err := driver.Acquire(context.Background(), backend.AcquireRequest{
		DefinitionID: "OrderLock",
		ResourceKeys: []string{resourceKey},
		OwnerID:      "owner-a",
		LeaseTTL:     leaseTTL,
	})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "owner-a: acquired %s\n", resourceKey); err != nil {
		return err
	}

	_, err = driver.Acquire(context.Background(), backend.AcquireRequest{
		DefinitionID: "OrderLock",
		ResourceKeys: []string{resourceKey},
		OwnerID:      "owner-b",
		LeaseTTL:     leaseTTL,
	})
	switch {
	case errors.Is(err, backend.ErrLeaseAlreadyHeld), errors.Is(err, lockerrors.ErrLockBusy):
		if _, writeErr := fmt.Fprintln(out, "owner-b before ttl: lock busy"); writeErr != nil {
			return writeErr
		}
	case err != nil:
		return err
	default:
		return fmt.Errorf("expected owner-b acquire before ttl to fail")
	}

	time.Sleep(leaseTTL + 20*time.Millisecond)

	leaseB, err := driver.Acquire(context.Background(), backend.AcquireRequest{
		DefinitionID: "OrderLock",
		ResourceKeys: []string{resourceKey},
		OwnerID:      "owner-b",
		LeaseTTL:     leaseTTL,
	})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "owner-b after ttl: acquired %s\n", resourceKey); err != nil {
		return err
	}

	if err := driver.Release(context.Background(), leaseB); err != nil {
		return err
	}

	// The original lease is already expired, so cleanup is best-effort only.
	_ = driver.Release(context.Background(), leaseA)

	if err := mgr.Shutdown(context.Background()); err != nil {
		return err
	}

	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}
