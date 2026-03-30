//go:build lockman_examples

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/lockkit/testkit"
)

type shipmentInput struct {
	ShipmentID string
}

var shipmentAggregateLock = lockman.DefineRun[shipmentInput](
	"ShipmentAggregateLock",
	lockman.BindResourceID("shipment", func(in shipmentInput) string { return in.ShipmentID }),
	lockman.TTL(30*time.Second),
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "example failed: %v\n", err)
		os.Exit(1)
	}
}

func run(out io.Writer) error {
	reg := lockman.NewRegistry()
	if err := reg.Register(shipmentAggregateLock); err != nil {
		return err
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "shipment-runtime"}),
		lockman.WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		return err
	}
	defer client.Shutdown(context.Background())

	req, err := shipmentAggregateLock.With(shipmentInput{ShipmentID: "sh-123"})
	if err != nil {
		return err
	}
	if err := client.Run(context.Background(), req, func(_ context.Context, lease lockman.Lease) error {
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
	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}
