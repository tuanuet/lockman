package inspect_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/observe"
)

// ---------------------------------------------------------------------------
// Store.Consume → runtime state materialisation
// ---------------------------------------------------------------------------

func TestStoreConsumeAppliesRuntimeAcquireWithoutBreakingHistory(t *testing.T) {
	store := inspect.NewStore(inspect.WithHistoryLimit(2))

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:123",
		OwnerID:      "orders-api",
		Timestamp:    time.Now(),
	})

	snap := store.Snapshot()
	if len(snap.RuntimeLocks) != 1 {
		t.Fatalf("expected 1 runtime lock, got %d", len(snap.RuntimeLocks))
	}
	lock := snap.RuntimeLocks[0]
	if lock.DefinitionID != "order.approve" {
		t.Fatalf("expected DefinitionID=order.approve, got %s", lock.DefinitionID)
	}
	if lock.ResourceID != "order:123" {
		t.Fatalf("expected ResourceID=order:123, got %s", lock.ResourceID)
	}
	if lock.OwnerID != "orders-api" {
		t.Fatalf("expected OwnerID=orders-api, got %s", lock.OwnerID)
	}
}

func TestStoreConsumeAppliesRuntimeReleaseWithoutBreakingHistory(t *testing.T) {
	store := inspect.NewStore(inspect.WithHistoryLimit(2))

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:123",
		OwnerID:      "orders-api",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventReleased,
		DefinitionID: "order.approve",
		ResourceID:   "order:123",
		OwnerID:      "orders-api",
		Timestamp:    time.Now(),
	})

	snap := store.Snapshot()
	if len(snap.RuntimeLocks) != 0 {
		t.Fatalf("expected no active runtime locks, got %#v", snap.RuntimeLocks)
	}
	// History still contains both events.
	if len(store.RecentEvents(10)) != 2 {
		t.Fatalf("expected 2 events in history, got %d", len(store.RecentEvents(10)))
	}
}

func TestStoreConsumeMaterialisesWorkerClaim(t *testing.T) {
	store := inspect.NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireStarted,
		DefinitionID: "payment.process",
		ResourceID:   "payment:456",
		OwnerID:      "payments-worker",
		Timestamp:    time.Now(),
	})

	snap := store.Snapshot()
	if len(snap.WorkerClaims) != 1 {
		t.Fatalf("expected 1 worker claim, got %d", len(snap.WorkerClaims))
	}
	claim := snap.WorkerClaims[0]
	if claim.DefinitionID != "payment.process" {
		t.Fatalf("expected DefinitionID=payment.process, got %s", claim.DefinitionID)
	}
}

func TestStoreConsumeMaterialisesRenewalInfo(t *testing.T) {
	store := inspect.NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:123",
		OwnerID:      "orders-api",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventRenewalSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:123",
		OwnerID:      "orders-api",
		Timestamp:    time.Now(),
	})

	snap := store.Snapshot()
	if len(snap.Renewals) != 1 {
		t.Fatalf("expected 1 renewal, got %d", len(snap.Renewals))
	}
}

func TestStoreConsumeMaterialisesShutdownState(t *testing.T) {
	store := inspect.NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:      observe.EventShutdownStarted,
		Timestamp: time.Now(),
	})

	snap := store.Snapshot()
	if !snap.Shutdown.Started {
		t.Fatal("expected Shutdown.Started=true")
	}
	if snap.Shutdown.Completed {
		t.Fatal("expected Shutdown.Completed=false after only ShutdownStarted")
	}

	_ = store.Consume(context.Background(), observe.Event{
		Kind:      observe.EventShutdownCompleted,
		Timestamp: time.Now(),
	})

	snap = store.Snapshot()
	if !snap.Shutdown.Completed {
		t.Fatal("expected Shutdown.Completed=true")
	}
}

func TestStoreConsumeHandlesLeaseLost(t *testing.T) {
	store := inspect.NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:123",
		OwnerID:      "orders-api",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventLeaseLost,
		DefinitionID: "order.approve",
		ResourceID:   "order:123",
		OwnerID:      "orders-api",
		Timestamp:    time.Now(),
	})

	snap := store.Snapshot()
	if len(snap.RuntimeLocks) != 0 {
		t.Fatalf("expected no runtime locks after lease lost, got %d", len(snap.RuntimeLocks))
	}
}

// ---------------------------------------------------------------------------
// Event history ring buffer truncation
// ---------------------------------------------------------------------------

func TestRecentEventsTruncatesWithOldestFirstDrop(t *testing.T) {
	store := inspect.NewStore(inspect.WithHistoryLimit(3))

	for i := 0; i < 5; i++ {
		_ = store.Consume(context.Background(), observe.Event{
			Kind:         observe.EventAcquireStarted,
			DefinitionID: "lock",
			ResourceID:   "res",
			OwnerID:      "owner",
			Timestamp:    time.Now().Add(time.Duration(i) * time.Millisecond),
		})
	}

	recent := store.RecentEvents(10)
	if len(recent) != 3 {
		t.Fatalf("expected 3 events (history limit), got %d", len(recent))
	}
	// Oldest should be event index 2 (the 3rd event), since first two were dropped.
	if recent[0].Timestamp.After(recent[1].Timestamp) {
		t.Fatal("expected oldest-first ordering within the ring buffer")
	}
}

// ---------------------------------------------------------------------------
// Query filters
// ---------------------------------------------------------------------------

func TestQueryByDefinitionID(t *testing.T) {
	store := inspect.NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:1",
		OwnerID:      "api",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "payment.process",
		ResourceID:   "pay:1",
		OwnerID:      "worker",
		Timestamp:    time.Now(),
	})

	results := store.Query(inspect.QueryOptions{DefinitionID: "order.approve"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for DefinitionID filter, got %d", len(results))
	}
}

func TestQueryByResourceID(t *testing.T) {
	store := inspect.NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:1",
		OwnerID:      "api",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:2",
		OwnerID:      "api",
		Timestamp:    time.Now(),
	})

	results := store.Query(inspect.QueryOptions{ResourceID: "order:1"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for ResourceID filter, got %d", len(results))
	}
}

func TestQueryByOwnerID(t *testing.T) {
	store := inspect.NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:1",
		OwnerID:      "api-a",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:2",
		OwnerID:      "api-b",
		Timestamp:    time.Now(),
	})

	results := store.Query(inspect.QueryOptions{OwnerID: "api-b"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for OwnerID filter, got %d", len(results))
	}
}

func TestQueryByKind(t *testing.T) {
	store := inspect.NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock",
		ResourceID:   "res",
		OwnerID:      "owner",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventReleased,
		DefinitionID: "lock",
		ResourceID:   "res",
		OwnerID:      "owner",
		Timestamp:    time.Now(),
	})

	results := store.Query(inspect.QueryOptions{Kind: observe.EventReleased})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for Kind filter, got %d", len(results))
	}
}

func TestQueryByTimeWindow(t *testing.T) {
	store := inspect.NewStore()
	now := time.Now()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock",
		ResourceID:   "res",
		OwnerID:      "owner",
		Timestamp:    now.Add(-2 * time.Hour),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock",
		ResourceID:   "res",
		OwnerID:      "owner",
		Timestamp:    now,
	})

	results := store.Query(inspect.QueryOptions{
		Since: now.Add(-1 * time.Hour),
		Until: now.Add(1 * time.Hour),
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for time window filter, got %d", len(results))
	}
}

func TestQueryCombinedFilters(t *testing.T) {
	store := inspect.NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:1",
		OwnerID:      "api",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireFailed,
		DefinitionID: "order.approve",
		ResourceID:   "order:2",
		OwnerID:      "api",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "payment.process",
		ResourceID:   "pay:1",
		OwnerID:      "worker",
		Timestamp:    time.Now(),
	})

	results := store.Query(inspect.QueryOptions{
		DefinitionID: "order.approve",
		Kind:         observe.EventAcquireSucceeded,
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for combined filter, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// Subscriptions
// ---------------------------------------------------------------------------

func TestSubscribeReceivesFutureEvents(t *testing.T) {
	store := inspect.NewStore()
	ch := make(chan observe.Event, 10)
	unsub := store.Subscribe(ch)
	defer unsub()

	evt := observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock",
		ResourceID:   "res",
		OwnerID:      "owner",
		Timestamp:    time.Now(),
	}
	_ = store.Consume(context.Background(), evt)

	select {
	case got := <-ch:
		if got.DefinitionID != evt.DefinitionID {
			t.Fatalf("expected DefinitionID=%s, got %s", evt.DefinitionID, got.DefinitionID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for subscribed event")
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	store := inspect.NewStore()
	ch := make(chan observe.Event, 10)
	unsub := store.Subscribe(ch)

	evt := observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock",
		ResourceID:   "res",
		OwnerID:      "owner",
		Timestamp:    time.Now(),
	}
	_ = store.Consume(context.Background(), evt)

	// Drain the channel.
	<-ch

	// Unsubscribe and send another event.
	unsub()
	_ = store.Consume(context.Background(), evt)

	select {
	case <-ch:
		t.Fatal("expected no event after unsubscribe")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestSlowSubscriberDoesNotBlockConsume(t *testing.T) {
	store := inspect.NewStore()
	// Very small channel; the non-blocking send should drop if subscriber is slow.
	ch := make(chan observe.Event, 1)

	// Don't drain the channel—let it fill.
	store.Subscribe(ch)

	evt := observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock",
		ResourceID:   "res",
		OwnerID:      "owner",
		Timestamp:    time.Now(),
	}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			_ = store.Consume(context.Background(), evt)
		}
		close(done)
	}()

	select {
	case <-done:
		// Consume returned even though subscriber is blocked.
	case <-time.After(2 * time.Second):
		t.Fatal("Consume blocked on slow subscriber")
	}
}

// ---------------------------------------------------------------------------
// Pipeline state
// ---------------------------------------------------------------------------

func TestUpdatePipelineStateAppearsInSnapshot(t *testing.T) {
	store := inspect.NewStore()

	store.UpdatePipelineState(inspect.PipelineState{
		DropPolicy:           "drop_oldest",
		BufferSize:           512,
		DroppedCount:         42,
		SinkFailureCount:     3,
		ExporterFailureCount: 1,
	})

	snap := store.Snapshot()
	if snap.Pipeline.DroppedCount != 42 {
		t.Fatalf("expected DroppedCount=42, got %d", snap.Pipeline.DroppedCount)
	}
}

// ---------------------------------------------------------------------------
// Concurrent access safety
// ---------------------------------------------------------------------------

func TestConcurrentConsumeAndSnapshot(t *testing.T) {
	store := inspect.NewStore()
	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = store.Consume(context.Background(), observe.Event{
					Kind:         observe.EventAcquireSucceeded,
					DefinitionID: "lock",
					ResourceID:   "res",
					OwnerID:      "owner",
					Timestamp:    time.Now(),
				})
			}
		}()
	}

	// Readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = store.Snapshot()
				_ = store.RecentEvents(50)
				_ = store.Query(inspect.QueryOptions{DefinitionID: "lock"})
			}
		}()
	}

	wg.Wait()
}
