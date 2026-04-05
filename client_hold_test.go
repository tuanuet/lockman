package lockman

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend/memory"
	"github.com/tuanuet/lockman/internal/sdk"
)

func testHoldUseCaseHelper(name string) HoldUseCase[string] {
	def := DefineLock(name, BindResourceID("order", func(v string) string { return v }))
	return DefineHoldOn[string](
		name,
		def,
		TTL(15*time.Minute),
	)
}

func mustNewHoldClient(t *testing.T, reg *Registry) *Client {
	t.Helper()

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "holder-1"}),
		WithBackend(memory.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return client
}

func TestHoldReturnsHandleWithToken(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCaseHelper("order.hold")
	mustRegisterUseCases(t, reg, uc)
	client := mustNewHoldClient(t, reg)

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	handle, err := client.Hold(context.Background(), req)
	if err != nil {
		t.Fatalf("Hold returned error: %v", err)
	}
	if handle.Token() == "" {
		t.Fatal("expected non-empty hold token")
	}
}

func TestHoldTokenHasExpectedPrefix(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCaseHelper("order.hold")
	mustRegisterUseCases(t, reg, uc)
	client := mustNewHoldClient(t, reg)

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	handle, err := client.Hold(context.Background(), req)
	if err != nil {
		t.Fatalf("Hold returned error: %v", err)
	}
	if !strings.HasPrefix(handle.Token(), "h1_") {
		t.Fatalf("expected token prefix h1_, got %q", handle.Token())
	}
}

func TestForfeitReleasesHold(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCaseHelper("order.hold")
	mustRegisterUseCases(t, reg, uc)
	client := mustNewHoldClient(t, reg)

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	first, err := client.Hold(context.Background(), req)
	if err != nil {
		t.Fatalf("first Hold returned error: %v", err)
	}

	if err := client.Forfeit(context.Background(), uc.ForfeitWith(first.Token())); err != nil {
		t.Fatalf("Forfeit returned error: %v", err)
	}

	if _, err := client.Hold(context.Background(), req); err != nil {
		t.Fatalf("second Hold returned error: %v", err)
	}
}

func TestHoldRejectsRequestFromDifferentRegistry(t *testing.T) {
	regA := NewRegistry()
	regB := NewRegistry()
	ucA := testHoldUseCaseHelper("order.hold")
	ucB := testHoldUseCaseHelper("order.hold.other")
	mustRegisterUseCases(t, regA, ucA)
	mustRegisterUseCases(t, regB, ucB)
	client := mustNewHoldClient(t, regA)

	req, err := ucB.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	_, err = client.Hold(context.Background(), req)
	if !errors.Is(err, ErrRegistryMismatch) {
		t.Fatalf("expected ErrRegistryMismatch, got %v", err)
	}
}

func TestHoldRejectsUnregisteredUseCase(t *testing.T) {
	reg := NewRegistry()
	registered := testHoldUseCaseHelper("order.hold")
	unregistered := testHoldUseCaseHelper("order.hold.unregistered")
	mustRegisterUseCases(t, reg, registered)
	client := mustNewHoldClient(t, reg)

	req, err := unregistered.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	_, err = client.Hold(context.Background(), req)
	if !errors.Is(err, ErrUseCaseNotFound) {
		t.Fatalf("expected ErrUseCaseNotFound, got %v", err)
	}
}

func TestForfeitWithMalformedTokenReturnsErrHoldTokenInvalid(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCaseHelper("order.hold")
	mustRegisterUseCases(t, reg, uc)
	client := mustNewHoldClient(t, reg)

	err := client.Forfeit(context.Background(), uc.ForfeitWith("not-a-token"))
	if !errors.Is(err, ErrHoldTokenInvalid) {
		t.Fatalf("expected ErrHoldTokenInvalid, got %v", err)
	}
}

func TestForfeitAlreadyReleasedLeaseReturnsErrHoldExpired(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCaseHelper("order.hold")
	mustRegisterUseCases(t, reg, uc)
	client := mustNewHoldClient(t, reg)

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	handle, err := client.Hold(context.Background(), req)
	if err != nil {
		t.Fatalf("Hold returned error: %v", err)
	}

	releaseReq := uc.ForfeitWith(handle.Token())
	if err := client.Forfeit(context.Background(), releaseReq); err != nil {
		t.Fatalf("first Forfeit returned error: %v", err)
	}

	err = client.Forfeit(context.Background(), releaseReq)
	if !errors.Is(err, ErrHoldExpired) {
		t.Fatalf("expected ErrHoldExpired, got %v", err)
	}
}

func TestForfeitRejectsUnregisteredUseCase(t *testing.T) {
	reg := NewRegistry()
	registered := testHoldUseCaseHelper("order.hold")
	unregistered := testHoldUseCaseHelper("order.hold.unregistered")
	mustRegisterUseCases(t, reg, registered)
	client := mustNewHoldClient(t, reg)

	token, err := sdk.EncodeHoldToken([]string{"order:123"}, "holder-1")
	if err != nil {
		t.Fatalf("EncodeHoldToken returned error: %v", err)
	}

	err = client.Forfeit(context.Background(), unregistered.ForfeitWith(token))
	if !errors.Is(err, ErrUseCaseNotFound) {
		t.Fatalf("expected ErrUseCaseNotFound, got %v", err)
	}
}

func TestHoldEncodeFailureDoesNotLeakLease(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCaseHelper("order.hold")
	mustRegisterUseCases(t, reg, uc)
	client := mustNewHoldClient(t, reg)

	tooLongOwnerID := strings.Repeat("a", 1<<16)

	req, err := uc.With("123", OwnerID(tooLongOwnerID))
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	_, err = client.Hold(context.Background(), req)
	if !errors.Is(err, ErrHoldTokenInvalid) {
		t.Fatalf("expected ErrHoldTokenInvalid, got %v", err)
	}

	retryReq, err := uc.With("123", OwnerID("holder-2"))
	if err != nil {
		t.Fatalf("retry With returned error: %v", err)
	}

	if _, err := client.Hold(context.Background(), retryReq); err != nil {
		t.Fatalf("expected retry Hold to succeed after encode failure, got %v", err)
	}
}
