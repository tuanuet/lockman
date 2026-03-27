package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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

	registerComposite := func(id string, members []string) error {
		return reg.RegisterComposite(definitions.CompositeDefinition{
			ID:               id,
			Members:          members,
			OrderingPolicy:   definitions.OrderingCanonical,
			AcquirePolicy:    definitions.AcquireAllOrNothing,
			EscalationPolicy: definitions.EscalationReject,
			ModeResolution:   definitions.ModeResolutionHomogeneous,
			MaxMemberCount:   2,
			ExecutionKind:    definitions.ExecutionSync,
		})
	}
	if err := registerComposite("OrderParentThenChild", []string{"OrderParentLock", "OrderItemLock"}); err != nil {
		return err
	}
	if err := registerComposite("OrderChildThenParent", []string{"OrderItemLock", "OrderParentLock"}); err != nil {
		return err
	}

	mgr, err := runtime.NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		return err
	}

	scenarios := []struct {
		label        string
		definitionID string
		memberInputs []map[string]string
	}{
		{
			label:        "scenario parent-then-child",
			definitionID: "OrderParentThenChild",
			memberInputs: []map[string]string{
				{"order_id": "123"},
				{"order_id": "123", "item_id": "line-1"},
			},
		},
		{
			label:        "scenario child-then-parent",
			definitionID: "OrderChildThenParent",
			memberInputs: []map[string]string{
				{"order_id": "123", "item_id": "line-1"},
				{"order_id": "123"},
			},
		},
	}

	for _, scenario := range scenarios {
		err := mgr.ExecuteCompositeExclusive(context.Background(), definitions.CompositeLockRequest{
			DefinitionID: scenario.definitionID,
			MemberInputs: scenario.memberInputs,
			Ownership: definitions.OwnershipMeta{
				OwnerID:     "example:parent-child-runtime",
				ServiceName: "example",
				HandlerName: scenario.label,
			},
		}, func(ctx context.Context, lease definitions.LeaseContext) error {
			return errors.New("callback should not run")
		})
		switch {
		case errors.Is(err, lockerrors.ErrPolicyViolation):
			if _, err := fmt.Fprintf(out, "%s: rejected\n", scenario.label); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			return fmt.Errorf("expected overlap rejection for %s", scenario.label)
		}
	}

	if _, err := fmt.Fprintln(out, "note: runtime overlap rejection is demonstrated through declared composite plans"); err != nil {
		return err
	}

	if err := mgr.Shutdown(context.Background()); err != nil {
		return err
	}

	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}
