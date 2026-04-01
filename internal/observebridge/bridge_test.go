package observebridge_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/internal/observebridge"
	"github.com/tuanuet/lockman/observe"
)

// stubStore is a minimal inspect.Store stand-in for testing.
type stubStore struct {
	consumeFn func(context.Context, observe.Event) error
	calls     atomic.Int64
}

func (s *stubStore) Consume(ctx context.Context, event observe.Event) error {
	s.calls.Add(1)
	if s.consumeFn != nil {
		return s.consumeFn(ctx, event)
	}
	return nil
}

// stubDispatcher records Publish calls.
type stubDispatcher struct {
	publishFn func(observe.Event)
	calls     atomic.Int64
	dropped   atomic.Int64
	sinkFail  atomic.Int64
	expFail   atomic.Int64
}

func (d *stubDispatcher) Publish(event observe.Event) {
	d.calls.Add(1)
	if d.publishFn != nil {
		d.publishFn(event)
	}
}

func (d *stubDispatcher) Shutdown(ctx context.Context) error { return nil }
func (d *stubDispatcher) DroppedCount() int64                { return d.dropped.Load() }
func (d *stubDispatcher) SinkFailureCount() int64            { return d.sinkFail.Load() }
func (d *stubDispatcher) ExporterFailureCount() int64        { return d.expFail.Load() }

var _ observe.Dispatcher = (*stubDispatcher)(nil)

func TestBridgeWritesOnceToStoreAndOnceToDispatcher(t *testing.T) {
	store := &stubStore{}
	dispatcher := &stubDispatcher{}

	bridge := observebridge.New(observebridge.Config{
		Store:      store,
		Dispatcher: dispatcher,
	})

	bridge.PublishRuntimeAcquireSucceeded(observe.Event{
		DefinitionID: "order.approve",
		ResourceID:   "order:42",
		OwnerID:      "owner-1",
	})

	if got := store.calls.Load(); got != 1 {
		t.Fatalf("store calls = %d, want 1", got)
	}
	if got := dispatcher.calls.Load(); got != 1 {
		t.Fatalf("dispatcher calls = %d, want 1", got)
	}
}

func TestBridgeAppliesLocalStateBeforePublish(t *testing.T) {
	var storeCalledFirst bool
	var dispatcherCalledFirst bool
	var sawStoreBeforeDispatcher atomic.Bool

	store := &stubStore{
		consumeFn: func(_ context.Context, _ observe.Event) error {
			if !dispatcherCalledFirst {
				storeCalledFirst = true
			}
			return nil
		},
	}
	dispatcher := &stubDispatcher{
		publishFn: func(_ observe.Event) {
			if storeCalledFirst {
				sawStoreBeforeDispatcher.Store(true)
			}
			dispatcherCalledFirst = true
		},
	}

	bridge := observebridge.New(observebridge.Config{
		Store:      store,
		Dispatcher: dispatcher,
	})

	bridge.PublishRuntimeAcquireSucceeded(observe.Event{
		DefinitionID: "order.approve",
		ResourceID:   "order:42",
		OwnerID:      "owner-1",
	})

	if !sawStoreBeforeDispatcher.Load() {
		t.Fatal("expected store to be called before dispatcher")
	}
}

func TestBridgeRuntimeLifecycleEvents(t *testing.T) {
	store := &stubStore{}
	dispatcher := &stubDispatcher{}

	bridge := observebridge.New(observebridge.Config{
		Store:      store,
		Dispatcher: dispatcher,
	})

	re := observe.Event{
		DefinitionID: "order.approve",
		ResourceID:   "order:42",
		OwnerID:      "owner-1",
	}

	bridge.PublishRuntimeAcquireStarted(re)
	bridge.PublishRuntimeAcquireSucceeded(re)
	bridge.PublishRuntimeAcquireFailed(re, context.DeadlineExceeded)
	bridge.PublishRuntimeReleased(re)
	bridge.PublishRuntimeRenewalSucceeded(re)
	bridge.PublishRuntimeRenewalFailed(re, context.DeadlineExceeded)
	bridge.PublishRuntimeLeaseLost(re)
	bridge.PublishRuntimeContention(re)

	if got := store.calls.Load(); got != 8 {
		t.Fatalf("store calls = %d, want 8", got)
	}
	if got := dispatcher.calls.Load(); got != 8 {
		t.Fatalf("dispatcher calls = %d, want 8", got)
	}
}

func TestBridgeWorkerLifecycleEvents(t *testing.T) {
	store := &stubStore{}
	dispatcher := &stubDispatcher{}

	bridge := observebridge.New(observebridge.Config{
		Store:      store,
		Dispatcher: dispatcher,
	})

	we := observe.Event{
		DefinitionID: "order.process",
		ResourceID:   "order:42",
		OwnerID:      "worker-1",
	}

	bridge.PublishWorkerAcquireStarted(we)
	bridge.PublishWorkerAcquireSucceeded(we)
	bridge.PublishWorkerAcquireFailed(we, context.DeadlineExceeded)
	bridge.PublishWorkerReleased(we)
	bridge.PublishWorkerOverlap(we)

	if got := store.calls.Load(); got != 5 {
		t.Fatalf("store calls = %d, want 5", got)
	}
	if got := dispatcher.calls.Load(); got != 5 {
		t.Fatalf("dispatcher calls = %d, want 5", got)
	}
}

func TestBridgeShutdownEvents(t *testing.T) {
	store := &stubStore{}
	dispatcher := &stubDispatcher{}

	bridge := observebridge.New(observebridge.Config{
		Store:      store,
		Dispatcher: dispatcher,
	})

	bridge.PublishRuntimeShutdownStarted()
	bridge.PublishRuntimeShutdownCompleted()

	if got := store.calls.Load(); got != 2 {
		t.Fatalf("store calls = %d, want 2", got)
	}
	if got := dispatcher.calls.Load(); got != 2 {
		t.Fatalf("dispatcher calls = %d, want 2", got)
	}
}

func TestBridgeRequestIDIsOptional(t *testing.T) {
	store := &stubStore{}
	dispatcher := &stubDispatcher{}

	bridge := observebridge.New(observebridge.Config{
		Store:      store,
		Dispatcher: dispatcher,
	})

	// Publish without RequestID should succeed.
	bridge.PublishRuntimeAcquireSucceeded(observe.Event{
		DefinitionID: "order.approve",
		ResourceID:   "order:42",
		OwnerID:      "owner-1",
	})

	if got := store.calls.Load(); got != 1 {
		t.Fatalf("store calls = %d, want 1", got)
	}
}

func TestBridgeRequestIDPropagatedWhenSet(t *testing.T) {
	var capturedEvent observe.Event

	store := &stubStore{
		consumeFn: func(_ context.Context, e observe.Event) error {
			capturedEvent = e
			return nil
		},
	}
	dispatcher := &stubDispatcher{}

	bridge := observebridge.New(observebridge.Config{
		Store:      store,
		Dispatcher: dispatcher,
	})

	bridge.PublishRuntimeAcquireSucceeded(observe.Event{
		DefinitionID: "order.approve",
		ResourceID:   "order:42",
		OwnerID:      "owner-1",
		RequestID:    "req-abc",
	})

	if capturedEvent.RequestID != "req-abc" {
		t.Fatalf("RequestID = %q, want %q", capturedEvent.RequestID, "req-abc")
	}
}

func TestBridgeShutdownHelperUsesDispatcherDeadline(t *testing.T) {
	slowDispatcher := &stubDispatcher{
		publishFn: func(_ observe.Event) {
			time.Sleep(200 * time.Millisecond)
		},
	}

	bridge := observebridge.New(observebridge.Config{
		Store:      &stubStore{},
		Dispatcher: slowDispatcher,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	_ = bridge.Shutdown(ctx)
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Fatalf("Shutdown took %v, expected to respect 10ms deadline", elapsed)
	}
}

func TestBridgeAllowsNilStore(t *testing.T) {
	b := observebridge.New(observebridge.Config{
		Store:      nil,
		Dispatcher: &stubDispatcher{},
	})
	if b == nil {
		t.Fatal("expected non-nil bridge with nil store")
	}
	// publish should not panic when store is nil
	b.PublishRuntimeAcquireStarted(observe.Event{Kind: observe.EventAcquireStarted})
}

func TestBridgeAllowsNilDispatcher(t *testing.T) {
	var consumeCount int
	b := observebridge.New(observebridge.Config{
		Store: &stubStore{
			consumeFn: func(_ context.Context, _ observe.Event) error {
				consumeCount++
				return nil
			},
		},
		Dispatcher: nil,
	})
	if b == nil {
		t.Fatal("expected non-nil bridge with nil dispatcher")
	}
	// publish should not panic when dispatcher is nil
	b.PublishRuntimeAcquireStarted(observe.Event{Kind: observe.EventAcquireStarted})
	if consumeCount != 1 {
		t.Fatalf("expected store to be called once, got %d", consumeCount)
	}
}

func TestBridgeShutdownWithNilDispatcher(t *testing.T) {
	b := observebridge.New(observebridge.Config{
		Store:      &stubStore{},
		Dispatcher: nil,
	})
	err := b.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("expected nil error with nil dispatcher, got %v", err)
	}
}

func TestBridgePublishesCorrectEventKind(t *testing.T) {
	var capturedKind observe.EventKind

	store := &stubStore{}
	dispatcher := &stubDispatcher{
		publishFn: func(e observe.Event) {
			capturedKind = e.Kind
		},
	}

	bridge := observebridge.New(observebridge.Config{
		Store:      store,
		Dispatcher: dispatcher,
	})

	tests := []struct {
		name string
		fn   func()
		want observe.EventKind
	}{
		{
			name: "acquire_started",
			fn: func() {
				bridge.PublishRuntimeAcquireStarted(observe.Event{
					DefinitionID: "d", ResourceID: "r", OwnerID: "o",
				})
			},
			want: observe.EventAcquireStarted,
		},
		{
			name: "acquire_succeeded",
			fn: func() {
				bridge.PublishRuntimeAcquireSucceeded(observe.Event{
					DefinitionID: "d", ResourceID: "r", OwnerID: "o",
				})
			},
			want: observe.EventAcquireSucceeded,
		},
		{
			name: "acquire_failed",
			fn: func() {
				bridge.PublishRuntimeAcquireFailed(observe.Event{
					DefinitionID: "d", ResourceID: "r", OwnerID: "o",
				}, assert.AnError)
			},
			want: observe.EventAcquireFailed,
		},
		{
			name: "released",
			fn: func() {
				bridge.PublishRuntimeReleased(observe.Event{
					DefinitionID: "d", ResourceID: "r", OwnerID: "o",
				})
			},
			want: observe.EventReleased,
		},
		{
			name: "renewal_succeeded",
			fn: func() {
				bridge.PublishRuntimeRenewalSucceeded(observe.Event{
					DefinitionID: "d", ResourceID: "r", OwnerID: "o",
				})
			},
			want: observe.EventRenewalSucceeded,
		},
		{
			name: "renewal_failed",
			fn: func() {
				bridge.PublishRuntimeRenewalFailed(observe.Event{
					DefinitionID: "d", ResourceID: "r", OwnerID: "o",
				}, assert.AnError)
			},
			want: observe.EventRenewalFailed,
		},
		{
			name: "lease_lost",
			fn: func() {
				bridge.PublishRuntimeLeaseLost(observe.Event{
					DefinitionID: "d", ResourceID: "r", OwnerID: "o",
				})
			},
			want: observe.EventLeaseLost,
		},
		{
			name: "contention",
			fn: func() {
				bridge.PublishRuntimeContention(observe.Event{
					DefinitionID: "d", ResourceID: "r", OwnerID: "o",
				})
			},
			want: observe.EventContention,
		},
		{
			name: "overlap_rejected",
			fn: func() {
				bridge.PublishRuntimeOverlapRejected(observe.Event{
					DefinitionID: "d", ResourceID: "r", OwnerID: "o",
				})
			},
			want: observe.EventOverlapRejected,
		},
		{
			name: "presence_checked",
			fn: func() {
				bridge.PublishRuntimePresenceChecked(observe.Event{
					DefinitionID: "d", ResourceID: "r", OwnerID: "o",
				})
			},
			want: observe.EventPresenceChecked,
		},
		{
			name: "shutdown_started",
			fn:   func() { bridge.PublishRuntimeShutdownStarted() },
			want: observe.EventShutdownStarted,
		},
		{
			name: "shutdown_completed",
			fn:   func() { bridge.PublishRuntimeShutdownCompleted() },
			want: observe.EventShutdownCompleted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn()
			if capturedKind != tt.want {
				t.Fatalf("event kind = %v, want %v", capturedKind, tt.want)
			}
		})
	}
}

func TestBridgeWithRealInspectStore(t *testing.T) {
	realStore := inspect.NewStore()
	dispatcher := &stubDispatcher{}

	bridge := observebridge.New(observebridge.Config{
		Store:      realStore,
		Dispatcher: dispatcher,
	})

	bridge.PublishRuntimeAcquireSucceeded(observe.Event{
		DefinitionID: "order.approve",
		ResourceID:   "order:42",
		OwnerID:      "owner-1",
	})

	snap := realStore.Snapshot()
	if len(snap.RuntimeLocks) != 1 {
		t.Fatalf("expected 1 runtime lock in snapshot, got %d", len(snap.RuntimeLocks))
	}
	lock := snap.RuntimeLocks[0]
	if lock.DefinitionID != "order.approve" {
		t.Fatalf("DefinitionID = %q, want %q", lock.DefinitionID, "order.approve")
	}
	if lock.ResourceID != "order:42" {
		t.Fatalf("ResourceID = %q, want %q", lock.ResourceID, "order:42")
	}
	if lock.OwnerID != "owner-1" {
		t.Fatalf("OwnerID = %q, want %q", lock.OwnerID, "owner-1")
	}
}

func TestBridgeWorkerOverlapEvent(t *testing.T) {
	var capturedKind observe.EventKind

	store := &stubStore{}
	dispatcher := &stubDispatcher{
		publishFn: func(e observe.Event) {
			capturedKind = e.Kind
		},
	}

	bridge := observebridge.New(observebridge.Config{
		Store:      store,
		Dispatcher: dispatcher,
	})

	bridge.PublishWorkerOverlap(observe.Event{
		DefinitionID: "order.process",
		ResourceID:   "order:42",
		OwnerID:      "worker-1",
	})

	if capturedKind != observe.EventOverlap {
		t.Fatalf("event kind = %v, want %v", capturedKind, observe.EventOverlap)
	}
}

var assert = struct {
	AnError error
}{
	AnError: assertAnError{},
}

type assertAnError struct{}

func (assertAnError) Error() string { return "assert.AnError" }
