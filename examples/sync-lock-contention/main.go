package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

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

func run(out io.Writer) error {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:               "OrderLock",
		Kind:             definitions.KindParent,
		Resource:         "order",
		Mode:             definitions.ModeStandard,
		ExecutionKind:    definitions.ExecutionSync,
		LeaseTTL:         30 * time.Second,
		CheckOnlyAllowed: true,
		KeyBuilder:       definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		return err
	}

	mgr, err := runtime.NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
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
			HandlerName: "ContentionFlow",
			OwnerID:     "example:local",
		},
	}

	held := make(chan struct{})
	release := make(chan struct{})

	firstErrCh := make(chan error, 1)
	go func() {
		req := req
		req.Ownership.OwnerID = "owner-a"
		firstErrCh <- mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
			if _, err := fmt.Fprintf(out, "goroutine owner-a: acquired %s\n", lease.ResourceKey); err != nil {
				return err
			}

			status, err := mgr.CheckPresence(ctx, definitions.PresenceCheckRequest{
				DefinitionID: req.DefinitionID,
				KeyInput:     req.KeyInput,
				Ownership:    req.Ownership,
			})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "presence while held: %s\n", presenceLabel(status.State)); err != nil {
				return err
			}

			close(held)
			<-release
			return nil
		})
	}()

	<-held

	var (
		secondErr error
		wg        sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		req := req
		req.Ownership.OwnerID = "owner-b"
		secondErr = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
			_, err := fmt.Fprintf(out, "goroutine owner-b: acquired %s\n", lease.ResourceKey)
			return err
		})
	}()

	wg.Wait()

	switch {
	case errors.Is(secondErr, lockerrors.ErrLockBusy):
		if _, err := fmt.Fprintln(out, "goroutine owner-b: lock busy"); err != nil {
			return err
		}
	case secondErr != nil:
		return secondErr
	}

	close(release)

	if err := <-firstErrCh; err != nil {
		return err
	}

	status, err := mgr.CheckPresence(context.Background(), definitions.PresenceCheckRequest{
		DefinitionID: req.DefinitionID,
		KeyInput:     req.KeyInput,
		Ownership:    req.Ownership,
	})
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(out, "presence after release: %s\n", presenceLabel(status.State)); err != nil {
		return err
	}

	if err := mgr.Shutdown(context.Background()); err != nil {
		return err
	}

	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}

func presenceLabel(state definitions.PresenceState) string {
	switch state {
	case definitions.PresenceHeld:
		return "held"
	case definitions.PresenceNotHeld:
		return "not_held"
	default:
		return "unknown"
	}
}
