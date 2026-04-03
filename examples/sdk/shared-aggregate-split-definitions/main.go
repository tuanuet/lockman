//go:build lockman_examples

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/idempotency"
	"github.com/tuanuet/lockman/lockkit/testkit"
)

type approvalInput struct {
	OrderID string
}

var orderApprovalDef = lockman.DefineLock(
	"order",
	lockman.BindResourceID("order", func(in approvalInput) string { return in.OrderID }),
)

var orderApprovalSync = lockman.DefineRunOn("OrderApprovalSync", orderApprovalDef)

var orderApprovalAsync = lockman.DefineClaimOn(
	"OrderApprovalAsync",
	orderApprovalDef,
	lockman.Idempotent(),
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "example failed: %v\n", err)
		os.Exit(1)
	}
}

func run(out io.Writer) error {
	reg := lockman.NewRegistry()
	if err := reg.Register(orderApprovalSync, orderApprovalAsync); err != nil {
		return err
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "example-app"}),
		lockman.WithBackend(testkit.NewMemoryDriver()),
		lockman.WithIdempotency(idempotency.NewMemoryStore()),
	)
	if err != nil {
		return err
	}
	defer client.Shutdown(context.Background())

	syncReq, err := orderApprovalSync.With(approvalInput{OrderID: "123"})
	if err != nil {
		return err
	}
	if err := client.Run(context.Background(), syncReq, func(_ context.Context, lease lockman.Lease) error {
		if _, err := fmt.Fprintf(out, "runtime path: acquired %s\n", lease.ResourceKey); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "runtime definition: OrderApprovalSync")
		return err
	}); err != nil {
		return err
	}

	claimReq, err := orderApprovalAsync.With(approvalInput{OrderID: "123"}, lockman.Delivery{
		MessageID:     "message-order-123",
		ConsumerGroup: "examples",
		Attempt:       1,
	})
	if err != nil {
		return err
	}
	if err := client.Claim(context.Background(), claimReq, func(_ context.Context, claim lockman.Claim) error {
		if _, err := fmt.Fprintf(out, "worker path: claimed %s\n", claim.ResourceKey); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "worker definition: OrderApprovalAsync")
		return err
	}); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "shared aggregate key: order:123"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "teaching point: split sync and async definitions can still share one aggregate boundary"); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}
