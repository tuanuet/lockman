package composite

import (
	"context"
	"strings"
	"testing"
	"time"

	"lockman"
	"lockman/lockkit/testkit"
)

type transferInput struct {
	AccountID string
	LedgerID  string
}

func TestCompositePackageExposesPublicRunUseCaseAuthoring(t *testing.T) {
	reg := lockman.NewRegistry()
	transfer := DefineRunWithOptions(
		"transfer.run",
		[]lockman.UseCaseOption{
			lockman.TTL(5 * time.Second),
		},
		DefineMember("account", lockman.BindResourceID("account", func(in transferInput) string { return in.AccountID })),
		DefineMember("ledger", lockman.BindResourceID("ledger", func(in transferInput) string { return in.LedgerID })),
	)
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
	transfer := DefineRunWithOptions(
		"transfer.strict",
		[]lockman.UseCaseOption{lockman.Strict()},
		DefineMember("account", lockman.BindResourceID("account", func(in transferInput) string { return in.AccountID })),
		DefineMember("ledger", lockman.BindResourceID("ledger", func(in transferInput) string { return in.LedgerID })),
	)
	if err := reg.Register(transfer); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	_, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "transfer-owner"}),
		lockman.WithBackend(testkit.NewMemoryDriver()),
	)
	if err == nil {
		t.Fatal("expected strict composite setup to fail")
	}
	if !strings.Contains(err.Error(), "composite") || !strings.Contains(err.Error(), "strict") {
		t.Fatalf("expected strict composite error, got %v", err)
	}
}
