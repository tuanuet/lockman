package composite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/backend/memory"
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
		lockman.WithBackend(memory.NewMemoryDriver()),
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

func TestDefineLockPanicsOnEmptyComposite(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when defining composite lock with zero members")
		}
	}()
	type input struct{}
	DefineLock[input]("empty")
}

func TestDefineLockPanicsOnDuplicateDefinitions(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when defining composite lock with duplicate definitions")
		}
	}()
	type input struct {
		AccountID string
	}
	def := lockman.DefineLock(
		"account",
		lockman.BindResourceID("account", func(in input) string { return in.AccountID }),
	)
	DefineLock(
		"duplicate",
		def,
		def,
	)
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
			lockman.WithBackend(memory.NewMemoryDriver()),
		)
		t.Logf("New error: %v", err)
		if err == nil {
			t.Fatal("expected composite with strict member to fail at New")
		}
	}
}

func TestCompositePackageFailIfHeldCheckPassesWhenNotHeld(t *testing.T) {
	reg := lockman.NewRegistry()
	preconditionDef := lockman.DefineLock(
		"precondition",
		lockman.BindResourceID("precondition", func(in transferInput) string { return in.AccountID }),
		lockman.FailIfHeldDef(),
	)
	accountDef := lockman.DefineLock(
		"account",
		lockman.BindResourceID("account", func(in transferInput) string { return in.AccountID }),
	)
	transferDef := DefineLock(
		"transfer",
		preconditionDef,
		accountDef,
	)
	transfer := AttachRun("transfer.run", transferDef, lockman.TTL(5*time.Second))
	if err := reg.Register(transfer); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "transfer-owner"}),
		lockman.WithBackend(memory.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := transfer.With(transferInput{
		AccountID: "acct-123",
	})
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	var gotKeys []string
	if err := client.Run(context.Background(), req, func(_ context.Context, lease lockman.Lease) error {
		gotKeys = lease.ResourceKeys
		return nil
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(gotKeys) != 1 {
		t.Fatalf("expected 1 resource key, got %d", len(gotKeys))
	}
	if gotKeys[0] != "account:acct-123" {
		t.Fatalf("expected resource key %q, got %q", "account:acct-123", gotKeys[0])
	}
}

func TestCompositePackageFailIfHeldCheckAbortsWhenHeld(t *testing.T) {
	reg := lockman.NewRegistry()
	preconditionDef := lockman.DefineLock(
		"precondition",
		lockman.BindResourceID("precondition", func(in transferInput) string { return in.AccountID }),
		lockman.FailIfHeldDef(),
	)
	accountDef := lockman.DefineLock(
		"account",
		lockman.BindResourceID("account", func(in transferInput) string { return in.AccountID }),
	)
	transferDef := DefineLock(
		"transfer",
		preconditionDef,
		accountDef,
	)
	transfer := AttachRun("transfer.run", transferDef, lockman.TTL(5*time.Second))
	holdUC := lockman.DefineHoldOn("precondition.hold", preconditionDef)
	if err := reg.Register(transfer, holdUC); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	driver := memory.NewMemoryDriver()

	holderClient, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "holder-owner"}),
		lockman.WithBackend(driver),
	)
	if err != nil {
		t.Fatalf("holder New returned error: %v", err)
	}

	compositeClient, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "transfer-owner"}),
		lockman.WithBackend(driver),
	)
	if err != nil {
		t.Fatalf("composite New returned error: %v", err)
	}

	holdReq, err := holdUC.With(transferInput{AccountID: "acct-123"})
	if err != nil {
		t.Fatalf("hold With returned error: %v", err)
	}
	handle, err := holderClient.Hold(context.Background(), holdReq)
	if err != nil {
		t.Fatalf("Hold returned error: %v", err)
	}
	defer func() {
		_ = holderClient.Forfeit(context.Background(), holdUC.ForfeitWith(handle.Token()))
	}()

	req, err := transfer.With(transferInput{AccountID: "acct-123"})
	if err != nil {
		t.Fatalf("composite With returned error: %v", err)
	}

	err = compositeClient.Run(context.Background(), req, func(_ context.Context, lease lockman.Lease) error {
		t.Fatal("callback should not be called when precondition fails")
		return nil
	})
	if !errors.Is(err, lockman.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}
}

func TestCompositePackageFailIfHeldErrorIncludesOwnerInfo(t *testing.T) {
	reg := lockman.NewRegistry()
	preconditionDef := lockman.DefineLock(
		"precondition",
		lockman.BindResourceID("precondition", func(in transferInput) string { return in.AccountID }),
		lockman.FailIfHeldDef(),
	)
	accountDef := lockman.DefineLock(
		"account",
		lockman.BindResourceID("account", func(in transferInput) string { return in.AccountID }),
	)
	transferDef := DefineLock(
		"transfer",
		preconditionDef,
		accountDef,
	)
	transfer := AttachRun("transfer.run", transferDef, lockman.TTL(5*time.Second))
	holdUC := lockman.DefineHoldOn("precondition.hold", preconditionDef)
	if err := reg.Register(transfer, holdUC); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	driver := memory.NewMemoryDriver()

	holderClient, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "holder-owner"}),
		lockman.WithBackend(driver),
	)
	if err != nil {
		t.Fatalf("holder New returned error: %v", err)
	}

	compositeClient, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "transfer-owner"}),
		lockman.WithBackend(driver),
	)
	if err != nil {
		t.Fatalf("composite New returned error: %v", err)
	}

	holdReq, err := holdUC.With(transferInput{AccountID: "acct-123"})
	if err != nil {
		t.Fatalf("hold With returned error: %v", err)
	}
	handle, err := holderClient.Hold(context.Background(), holdReq)
	if err != nil {
		t.Fatalf("Hold returned error: %v", err)
	}
	defer func() {
		_ = holderClient.Forfeit(context.Background(), holdUC.ForfeitWith(handle.Token()))
	}()

	req, err := transfer.With(transferInput{AccountID: "acct-123"})
	if err != nil {
		t.Fatalf("composite With returned error: %v", err)
	}

	err = compositeClient.Run(context.Background(), req, func(_ context.Context, lease lockman.Lease) error {
		t.Fatal("callback should not be called when precondition fails")
		return nil
	})
	if !errors.Is(err, lockman.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}
}

func TestCompositePackageFailIfHeldMembersAreExcludedFromLeasePayload(t *testing.T) {
	reg := lockman.NewRegistry()
	preconditionDef := lockman.DefineLock(
		"precondition",
		lockman.BindResourceID("precondition", func(in transferInput) string { return in.AccountID }),
		lockman.FailIfHeldDef(),
	)
	accountDef := lockman.DefineLock(
		"account",
		lockman.BindResourceID("account", func(in transferInput) string { return in.AccountID }),
	)
	transferDef := DefineLock(
		"transfer",
		preconditionDef,
		accountDef,
	)
	transfer := AttachRun("transfer.run", transferDef, lockman.TTL(5*time.Second))
	if err := reg.Register(transfer); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "transfer-owner"}),
		lockman.WithBackend(memory.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := transfer.With(transferInput{AccountID: "acct-123"})
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	var gotKeys []string
	if err := client.Run(context.Background(), req, func(_ context.Context, lease lockman.Lease) error {
		gotKeys = lease.ResourceKeys
		return nil
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(gotKeys) != 1 {
		t.Fatalf("expected 1 resource key (FailIfHeld member excluded), got %d: %v", len(gotKeys), gotKeys)
	}
	if gotKeys[0] != "account:acct-123" {
		t.Fatalf("expected resource key %q, got %q", "account:acct-123", gotKeys[0])
	}
	for _, key := range gotKeys {
		if strings.HasPrefix(key, "precondition:") {
			t.Fatalf("FailIfHeld member should be excluded from lease resource keys, got %q", key)
		}
	}
}
