package workers

import (
	"context"
	"errors"
	"testing"
	"time"

	"lockman/lockkit/definitions"
	lockerrors "lockman/lockkit/errors"
	"lockman/lockkit/idempotency"
	"lockman/lockkit/registry"
	"lockman/lockkit/testkit"
)

func TestNewManagerRejectsInvalidRegistry(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "BrokenAsyncLock",
		Kind:          definitions.KindParent,
		Resource:      "broken",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionAsync,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err := NewManager(reg, testkit.NewMemoryDriver(), idempotency.NewMemoryStore())
	if !errors.Is(err, lockerrors.ErrRegistryViolation) {
		t.Fatalf("expected registry violation, got %v", err)
	}
}

func TestNewManagerRequiresIdempotencyStoreWhenAsyncDefinitionNeedsIt(t *testing.T) {
	reg := newWorkerRegistryForTest(t, true)

	_, err := NewManager(reg, testkit.NewMemoryDriver(), nil)
	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for missing idempotency store, got %v", err)
	}
}

func TestShutdownStopsNewClaims(t *testing.T) {
	mgr := newWorkerManagerForTest(t)

	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	err := mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		return nil
	})
	if !errors.Is(err, lockerrors.ErrWorkerShuttingDown) {
		t.Fatalf("expected worker shutting down error, got %v", err)
	}
}

func TestShutdownWaitsForInFlightClaimExecutionToDrain(t *testing.T) {
	mgr := newWorkerManagerForTest(t)

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	execErrCh := make(chan error, 1)
	go func() {
		execErrCh <- mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
			close(callbackEntered)
			<-releaseCallback
			return nil
		})
	}()

	<-callbackEntered

	shutdownErrCh := make(chan error, 1)
	go func() {
		shutdownErrCh <- mgr.Shutdown(context.Background())
	}()

	select {
	case err := <-shutdownErrCh:
		t.Fatalf("Shutdown returned before in-flight claim drained: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	close(releaseCallback)

	if err := <-shutdownErrCh; err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	if err := <-execErrCh; err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}
}

func TestShutdownReturnsContextErrorWhenInFlightDoesNotDrain(t *testing.T) {
	mgr := newWorkerManagerForTest(t)

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	execErrCh := make(chan error, 1)
	go func() {
		execErrCh <- mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
			close(callbackEntered)
			<-releaseCallback
			return nil
		})
	}()

	<-callbackEntered

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := mgr.Shutdown(shutdownCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}

	close(releaseCallback)
	if err := <-execErrCh; err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown should succeed after drain, got %v", err)
	}
}

func TestShutdownIsIdempotent(t *testing.T) {
	mgr := newWorkerManagerForTest(t)

	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown returned error: %v", err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown should be idempotent, got %v", err)
	}
}
