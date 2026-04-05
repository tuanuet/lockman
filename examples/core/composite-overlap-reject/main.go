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
		ID:            "OrderParentLock",
		Kind:          backend.KindParent,
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
		Kind:          backend.KindChild,
		Resource:      "order_item",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          20,
		ParentRef:     "OrderParentLock",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
	}); err != nil {
		return err
	}
	if err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "OrderComposite",
		Members:          []string{"OrderParentLock", "OrderItemLock"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   2,
		ExecutionKind:    definitions.ExecutionSync,
	}); err != nil {
		return err
	}

	mgr, err := runtime.NewManager(reg, memory.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		return err
	}

	req := definitions.CompositeLockRequest{
		DefinitionID: "OrderComposite",
		MemberInputs: []map[string]string{
			{"order_id": "123"},
			{"order_id": "123", "item_id": "line-1"},
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "example:overlap-reject"},
	}

	err = mgr.ExecuteCompositeExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		return errors.New("callback should not run")
	})
	switch {
	case errors.Is(err, lockerrors.ErrPolicyViolation):
		if _, err := fmt.Fprintln(out, "overlap outcome: rejected"); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		return fmt.Errorf("expected overlap rejection")
	}

	if err := mgr.Shutdown(context.Background()); err != nil {
		return err
	}

	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}
