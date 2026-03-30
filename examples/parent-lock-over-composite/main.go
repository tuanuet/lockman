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
		ID:            "ShipmentAggregateLock",
		Kind:          definitions.KindParent,
		Resource:      "shipment",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("shipment:{shipment_id}", []string{"shipment_id"}),
	}); err != nil {
		return err
	}

	mgr, err := runtime.NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		return err
	}
	defer func() {
		_ = mgr.Shutdown(context.Background())
	}()

	req := definitions.SyncLockRequest{
		DefinitionID: "ShipmentAggregateLock",
		KeyInput: map[string]string{
			"shipment_id": "sh-123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:     "example:shipment-runtime",
			ServiceName: "example",
			HandlerName: "ShipmentAggregateLock",
		},
	}

	if err := mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		if _, err := fmt.Fprintf(out, "aggregate lock: %s\n", lease.ResourceKey); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "sub-resources involved: package-1,package-2")
		return err
	}); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "teaching point: parent lock is enough, composite is overkill"); err != nil {
		return err
	}

	if err := mgr.Shutdown(context.Background()); err != nil {
		return err
	}

	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}
