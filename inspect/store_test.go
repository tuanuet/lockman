package inspect

import (
	"context"
	"testing"
	"time"

	"github.com/tuanuet/lockman/observe"
)

func TestStoreConsumeAppliesRuntimeReleaseWithoutBreakingHistory(t *testing.T) {
	store := NewStore(WithHistoryLimit(2))

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:123",
		OwnerID:      "orders-api",
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventReleased,
		DefinitionID: "order.approve",
		ResourceID:   "order:123",
		OwnerID:      "orders-api",
	})

	snap := store.Snapshot()
	if len(snap.RuntimeLocks) != 0 {
		t.Fatalf("expected no active runtime locks, got %#v", snap.RuntimeLocks)
	}
}

func TestStoreConsumeCapturesActiveLocks(t *testing.T) {
	store := NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:123",
		OwnerID:      "orders-api",
		Timestamp:    time.Now(),
	})

	snap := store.Snapshot()
	if len(snap.RuntimeLocks) != 1 {
		t.Fatalf("expected 1 active runtime lock, got %d", len(snap.RuntimeLocks))
	}
	if snap.RuntimeLocks[0].DefinitionID != "order.approve" {
		t.Fatalf("expected lock def id order.approve, got %s", snap.RuntimeLocks[0].DefinitionID)
	}
}

func TestStoreConsumeCapturesWorkerClaims(t *testing.T) {
	store := NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:123",
		OwnerID:      "worker-1",
		Timestamp:    time.Now(),
	})

	snap := store.Snapshot()
	if len(snap.WorkerClaims) != 1 {
		t.Fatalf("expected 1 worker claim, got %d", len(snap.WorkerClaims))
	}
}

func TestStoreConsumeCapturesRenewals(t *testing.T) {
	store := NewStore()

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

func TestStoreConsumeCapturesShutdownState(t *testing.T) {
	store := NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventShutdownStarted,
		DefinitionID: "order.approve",
		OwnerID:      "orders-api",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventShutdownCompleted,
		DefinitionID: "order.approve",
		OwnerID:      "orders-api",
		Timestamp:    time.Now(),
	})

	snap := store.Snapshot()
	if len(snap.Shutdowns) != 2 {
		t.Fatalf("expected 2 shutdown events, got %d", len(snap.Shutdowns))
	}
}

func TestStoreHistoryTruncatesOldestFirst(t *testing.T) {
	store := NewStore(WithHistoryLimit(2))

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock1",
		ResourceID:   "res:1",
		Timestamp:    time.Now().Add(-2 * time.Second),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock2",
		ResourceID:   "res:2",
		Timestamp:    time.Now().Add(-1 * time.Second),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock3",
		ResourceID:   "res:3",
		Timestamp:    time.Now(),
	})

	events := store.RecentEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events (truncated), got %d", len(events))
	}
}

func TestStoreQueryFilterByLockID(t *testing.T) {
	store := NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock1",
		ResourceID:   "res:1",
		OwnerID:      "owner1",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock2",
		ResourceID:   "res:2",
		OwnerID:      "owner2",
		Timestamp:    time.Now(),
	})

	events := store.Query(QueryFilter{LockID: "lock1"})
	if len(events) != 1 {
		t.Fatalf("expected 1 event for lock1, got %d", len(events))
	}
}

func TestStoreQueryFilterByResourceKey(t *testing.T) {
	store := NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock1",
		ResourceID:   "order:123",
		OwnerID:      "owner1",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock1",
		ResourceID:   "order:456",
		OwnerID:      "owner2",
		Timestamp:    time.Now(),
	})

	events := store.Query(QueryFilter{ResourceKey: "order:123"})
	if len(events) != 1 {
		t.Fatalf("expected 1 event for order:123, got %d", len(events))
	}
}

func TestStoreQueryFilterByOwnerID(t *testing.T) {
	store := NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock1",
		ResourceID:   "res:1",
		OwnerID:      "owner1",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock1",
		ResourceID:   "res:2",
		OwnerID:      "owner2",
		Timestamp:    time.Now(),
	})

	events := store.Query(QueryFilter{OwnerID: "owner1"})
	if len(events) != 1 {
		t.Fatalf("expected 1 event for owner1, got %d", len(events))
	}
}

func TestStoreQueryFilterByKind(t *testing.T) {
	store := NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock1",
		ResourceID:   "res:1",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventReleased,
		DefinitionID: "lock1",
		ResourceID:   "res:1",
		Timestamp:    time.Now(),
	})

	events := store.Query(QueryFilter{Kind: observe.EventAcquireSucceeded})
	if len(events) != 1 {
		t.Fatalf("expected 1 acquire event, got %d", len(events))
	}
}

func TestStoreQueryFilterByTimeWindow(t *testing.T) {
	store := NewStore()

	now := time.Now()
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock1",
		ResourceID:   "res:1",
		Timestamp:    now.Add(-2 * time.Hour),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock2",
		ResourceID:   "res:2",
		Timestamp:    now,
	})

	events := store.Query(QueryFilter{
		Since: now.Add(-1 * time.Hour),
	})
	if len(events) != 1 {
		t.Fatalf("expected 1 event in time window, got %d", len(events))
	}
}

func TestStoreSlowSubscriberDoesNotBlockConsume(t *testing.T) {
	store := NewStore()

	slowCh := make(chan observe.Event, 1)
	store.Subscribe(func(event observe.Event) error {
		select {
		case slowCh <- event:
		default:
		}
		return nil
	})

	done := make(chan struct{})
	go func() {
		_ = store.Consume(context.Background(), observe.Event{
			Kind:         observe.EventAcquireSucceeded,
			DefinitionID: "lock1",
			ResourceID:   "res:1",
			Timestamp:    time.Now(),
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Consume blocked on slow subscriber")
	}
}

func TestStoreSnapshotReturnsConsistentState(t *testing.T) {
	store := NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock1",
		ResourceID:   "res:1",
		OwnerID:      "owner1",
		Timestamp:    time.Now(),
	})

	snap1 := store.Snapshot()
	snap2 := store.Snapshot()

	if len(snap1.RuntimeLocks) != len(snap2.RuntimeLocks) {
		t.Fatalf("inconsistent snapshots: %d vs %d", len(snap1.RuntimeLocks), len(snap2.RuntimeLocks))
	}
}

func TestStorePipelineStateHealth(t *testing.T) {
	store := NewStore()

	store.UpdatePipelineState(PipelineState{
		Status:     "healthy",
		QueueDepth: 0,
		Errors:     0,
	})

	snap := store.Snapshot()
	if snap.Pipeline.Status != "healthy" {
		t.Fatalf("expected healthy status, got %s", snap.Pipeline.Status)
	}
}
