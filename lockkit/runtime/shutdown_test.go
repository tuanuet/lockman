package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/observe"
	"github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/lockkit/testkit"
)

func TestShutdownStopsNewAcquisitions(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	err = mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		return nil
	})
	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation after shutdown, got %v", err)
	}
}

func TestShutdownIsIdempotent(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown returned error: %v", err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown should be idempotent, got %v", err)
	}
}

func TestShutdownWaitsForInFlightExecutionToDrain(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	execErrCh := make(chan error, 1)
	go func() {
		execErrCh <- mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
			DefinitionID: "OrderLock",
			KeyInput: map[string]string{
				"order_id": "123",
			},
			Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
		}, func(ctx context.Context, lease definitions.LeaseContext) error {
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
		t.Fatalf("Shutdown returned before in-flight execution drained: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	close(releaseCallback)

	if err := <-shutdownErrCh; err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	if err := <-execErrCh; err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}
}

func TestShutdownWaitsForInFlightCompositeExecutionToDrain(t *testing.T) {
	reg := newCompositeRegistry(t)
	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	execErrCh := make(chan error, 1)
	go func() {
		execErrCh <- mgr.ExecuteCompositeExclusive(context.Background(), compositeRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
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
		t.Fatalf("Shutdown returned before in-flight composite execution drained: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	close(releaseCallback)

	if err := <-shutdownErrCh; err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	if err := <-execErrCh; err != nil {
		t.Fatalf("ExecuteCompositeExclusive returned error: %v", err)
	}
}

func TestShutdownReturnsContextErrorWhenInFlightDoesNotDrain(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	execErrCh := make(chan error, 1)
	go func() {
		execErrCh <- mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
			DefinitionID: "OrderLock",
			KeyInput: map[string]string{
				"order_id": "123",
			},
			Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
		}, func(ctx context.Context, lease definitions.LeaseContext) error {
			close(callbackEntered)
			<-releaseCallback
			return nil
		})
	}()

	<-callbackEntered

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err = mgr.Shutdown(shutdownCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected Shutdown to return context deadline exceeded, got %v", err)
	}

	close(releaseCallback)
	if err := <-execErrCh; err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown should succeed after in-flight execution drains, got %v", err)
	}
}

func TestShutdownWaitsForAcquireInProgressExecutionToDrain(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	driver := newBlockingDriver()
	mgr, err := NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	execErrCh := make(chan error, 1)
	go func() {
		execErrCh <- mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
			DefinitionID: "OrderLock",
			KeyInput: map[string]string{
				"order_id": "123",
			},
			Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
		}, func(ctx context.Context, lease definitions.LeaseContext) error {
			return nil
		})
	}()

	driver.WaitForAcquire()

	shutdownErrCh := make(chan error, 1)
	go func() {
		shutdownErrCh <- mgr.Shutdown(context.Background())
	}()

	select {
	case err := <-shutdownErrCh:
		t.Fatalf("Shutdown returned while acquire was still in progress: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	driver.UnblockAcquire()

	if err := <-execErrCh; err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}
	if err := <-shutdownErrCh; err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
}

func TestShutdownReturnsContextErrorWhenAcquireInProgressDoesNotDrain(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	driver := newBlockingDriver()
	mgr, err := NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	execErrCh := make(chan error, 1)
	go func() {
		execErrCh <- mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
			DefinitionID: "OrderLock",
			KeyInput: map[string]string{
				"order_id": "123",
			},
			Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
		}, func(ctx context.Context, lease definitions.LeaseContext) error {
			return nil
		})
	}()

	driver.WaitForAcquire()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err = mgr.Shutdown(shutdownCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected Shutdown to return context deadline exceeded, got %v", err)
	}

	driver.UnblockAcquire()

	if err := <-execErrCh; err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown should succeed after acquire-in-progress execution drains, got %v", err)
	}
}

func TestShutdownNotBlockedByReentrantAdmissionFailure(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	req := definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	}

	err = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		nestedErr := mgr.ExecuteExclusive(ctx, req, func(ctx context.Context, nested definitions.LeaseContext) error {
			return nil
		})
		if !errors.Is(nestedErr, lockerrors.ErrReentrantAcquire) {
			t.Fatalf("expected nested reentrant error, got %v", nestedErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := mgr.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown should not block on released reentrant admission, got %v", err)
	}
}

func TestShutdownEmitsBridgeLifecycleEvents(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	bridge := &bridgeStub{}
	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder(), WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if bridge.shutdownStarted != 1 {
		t.Fatalf("expected 1 shutdown started event, got %d", bridge.shutdownStarted)
	}
	if bridge.shutdownDone != 1 {
		t.Fatalf("expected 1 shutdown completed event, got %d", bridge.shutdownDone)
	}
}
