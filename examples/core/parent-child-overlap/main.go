package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

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
		ParentRef:     "OrderParentLock",
		OverlapPolicy: definitions.OverlapReject,
		Resource:      "order_item",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          20,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
	}); err != nil {
		return err
	}

	driver := memory.NewMemoryDriver()
	parentMgr, err := runtime.NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		return err
	}
	childMgr, err := runtime.NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		return err
	}
	defer func() {
		_ = childMgr.Shutdown(context.Background())
		_ = parentMgr.Shutdown(context.Background())
	}()

	if err := runScenario(
		out,
		"scenario child-held-parent-rejected",
		func(entered chan<- struct{}, release <-chan struct{}) error {
			return childMgr.ExecuteExclusive(context.Background(), childRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
				close(entered)
				<-release
				return nil
			})
		},
		func() error {
			return parentMgr.ExecuteExclusive(context.Background(), parentRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
				return errors.New("parent callback should not run")
			})
		},
	); err != nil {
		return err
	}

	if err := runScenario(
		out,
		"scenario parent-held-child-rejected",
		func(entered chan<- struct{}, release <-chan struct{}) error {
			return parentMgr.ExecuteExclusive(context.Background(), parentRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
				close(entered)
				<-release
				return nil
			})
		},
		func() error {
			return childMgr.ExecuteExclusive(context.Background(), childRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
				return errors.New("child callback should not run")
			})
		},
	); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "note: phase 2a runtime now enforces parent-child overlap across managers and goroutines"); err != nil {
		return err
	}

	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}

func runScenario(
	out io.Writer,
	label string,
	holder func(chan<- struct{}, <-chan struct{}) error,
	contender func() error,
) error {
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- holder(entered, release)
	}()
	<-entered

	err := contender()
	switch {
	case errors.Is(err, lockerrors.ErrOverlapRejected):
		if _, writeErr := fmt.Fprintf(out, "%s: overlap rejected\n", label); writeErr != nil {
			close(release)
			<-done
			return writeErr
		}
	case err != nil:
		close(release)
		<-done
		return err
	default:
		close(release)
		<-done
		return fmt.Errorf("expected overlap rejection for %s", label)
	}

	close(release)
	if err := <-done; err != nil {
		return err
	}
	return nil
}

func parentRequest() definitions.SyncLockRequest {
	return definitions.SyncLockRequest{
		DefinitionID: "OrderParentLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:     "example:parent",
			ServiceName: "example",
			HandlerName: "parent",
		},
	}
}

func childRequest() definitions.SyncLockRequest {
	return definitions.SyncLockRequest{
		DefinitionID: "OrderItemLock",
		KeyInput: map[string]string{
			"order_id": "123",
			"item_id":  "line-1",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:     "example:child",
			ServiceName: "example",
			HandlerName: "child",
		},
	}
}
