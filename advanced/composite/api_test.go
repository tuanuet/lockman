package composite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/lockkit/testkit"
)

type transferInput struct {
	AccountID string
	LedgerID  string
}

func TestCompositePackageExposesPublicRunUseCaseAuthoring(t *testing.T) {
	reg := lockman.NewRegistry()
	accountDef := lockman.DefineLock(
		"account",
		lockman.BindResourceID("account", func(in transferInput) string { return in.AccountID }),
	)
	ledgerDef := lockman.DefineLock(
		"ledger",
		lockman.BindResourceID("ledger", func(in transferInput) string { return in.LedgerID }),
	)
	transferDef := DefineLock(
		"transfer",
		accountDef,
		ledgerDef,
	)
	transfer := AttachRun("transfer.run", transferDef, lockman.TTL(5*time.Second))
	if err := reg.Register(transfer); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "transfer-owner"}),
		lockman.WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := transfer.With(transferInput{
		AccountID: "acct-123",
		LedgerID:  "ledger-456",
	})
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	var joined string
	if err := client.Run(context.Background(), req, func(_ context.Context, lease lockman.Lease) error {
		joined = strings.Join(lease.ResourceKeys, ",")
		if lease.LeaseTTL != 5*time.Second {
			t.Fatalf("expected ttl propagation, got %v", lease.LeaseTTL)
		}
		return nil
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if joined != "account:acct-123,ledger:ledger-456" {
		t.Fatalf("expected ordered resource keys, got %q", joined)
	}
}

func TestCompositePackageRejectsStrictCompositeRuns(t *testing.T) {
	reg := lockman.NewRegistry()
	strictAccountDef := lockman.DefineLock(
		"account",
		lockman.BindResourceID("account", func(in transferInput) string { return in.AccountID }),
		lockman.StrictDef(),
	)
	ledgerDef := lockman.DefineLock(
		"ledger",
		lockman.BindResourceID("ledger", func(in transferInput) string { return in.LedgerID }),
	)
	transferDef := DefineLock(
		"transfer",
		strictAccountDef,
		ledgerDef,
	)
	transfer := AttachRun("transfer.run", transferDef)
	err := reg.Register(transfer)
	t.Logf("Register error: %v", err)
	if err == nil {
		_, err := lockman.New(
			lockman.WithRegistry(reg),
			lockman.WithIdentity(lockman.Identity{OwnerID: "transfer-owner"}),
			lockman.WithBackend(testkit.NewMemoryDriver()),
		)
		t.Logf("New error: %v", err)
		if err == nil {
			t.Fatal("expected composite with strict member to fail at New")
		}
	}
}
