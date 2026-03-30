package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tuanuet/lockman/lockkit/definitions"
	"github.com/tuanuet/lockman/lockkit/observe"
	"github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/lockkit/runtime"
	"github.com/tuanuet/lockman/lockkit/testkit"
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "example failed: %v\n", err)
		os.Exit(1)
	}
}

func run(out io.Writer) error {
	reg := registry.New()
	register := func(def definitions.LockDefinition) error {
		return reg.Register(def)
	}

	if err := register(definitions.LockDefinition{
		ID:            "LedgerMember",
		Kind:          definitions.KindParent,
		Resource:      "ledger",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          20,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("ledger:{ledger_id}", []string{"ledger_id"}),
	}); err != nil {
		return err
	}
	if err := register(definitions.LockDefinition{
		ID:            "AccountMember",
		Kind:          definitions.KindParent,
		Resource:      "account",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          10,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("account:{account_id}", []string{"account_id"}),
	}); err != nil {
		return err
	}
	if err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "TransferComposite",
		Members:          []string{"LedgerMember", "AccountMember"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   2,
		ExecutionKind:    definitions.ExecutionSync,
	}); err != nil {
		return err
	}

	mgr, err := runtime.NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		return err
	}

	req := definitions.CompositeLockRequest{
		DefinitionID: "TransferComposite",
		MemberInputs: []map[string]string{
			{"ledger_id": "ledger-456"},
			{"account_id": "acct-123"},
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "example:composite-sync"},
	}

	if err := mgr.ExecuteCompositeExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		joined := strings.Join(lease.ResourceKeys, ",")
		if _, err := fmt.Fprintf(out, "composite acquired: %s\n", joined); err != nil {
			return err
		}
		if joined != "account:acct-123,ledger:ledger-456" {
			return fmt.Errorf("unexpected canonical order: %s", joined)
		}
		_, err := fmt.Fprintln(out, "canonical order: ok")
		return err
	}); err != nil {
		return err
	}

	if err := mgr.Shutdown(context.Background()); err != nil {
		return err
	}

	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}
