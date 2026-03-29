package idempotency_test

import (
	"context"
	"testing"
	"time"

	"lockman"
	"lockman/backend"
	"lockman/idempotency"
	"lockman/lockkit/registry"
	"lockman/lockkit/workers"
)

func TestStoreContractExists(t *testing.T) {
	// Compile-time assertion that the public Store contract exists.
	var _ idempotency.Store = idempotency.NewMemoryStore()
}

func TestClientCompilesAgainstPromotedIdempotency(t *testing.T) {
	// This doesn't need to run any client logic; it just pins the public option signature.
	_ = lockman.WithIdempotency(idempotency.NewMemoryStore())
}

func TestWorkerCompilesAgainstPromotedIdempotency(t *testing.T) {
	// Pin the worker manager constructor signature to the promoted contract.
	var _ func(reg registry.Reader, driver backend.Driver, store idempotency.Store) (*workers.Manager, error) = workers.NewManager
}

type fakeClock struct {
	current time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{current: start}
}

func (c *fakeClock) Now() time.Time {
	return c.current
}

func (c *fakeClock) Advance(delta time.Duration) {
	c.current = c.current.Add(delta)
}

func TestMemoryStoreBeginRejectsSecondActiveClaim(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 3, 26, 9, 0, 0, 0, time.UTC))
	store := idempotency.NewMemoryStoreWithNow(clock.Now)

	first, err := store.Begin(context.Background(), "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       1,
		TTL:           time.Minute,
	})
	if err != nil {
		t.Fatalf("first Begin returned error: %v", err)
	}
	if !first.Acquired {
		t.Fatal("expected first Begin to acquire slot")
	}

	second, err := store.Begin(context.Background(), "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-b",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       2,
		TTL:           time.Minute,
	})
	if err != nil {
		t.Fatalf("second Begin returned error: %v", err)
	}
	if second.Acquired {
		t.Fatal("expected duplicate Begin to be rejected")
	}
	if !second.Duplicate {
		t.Fatal("expected duplicate Begin result to have Duplicate=true")
	}
}

func TestMemoryStoreBeginAllowsReacquireAfterExpiryBoundary(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC))
	store := idempotency.NewMemoryStoreWithNow(clock.Now)

	first, err := store.Begin(context.Background(), "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       1,
		TTL:           time.Minute,
	})
	if err != nil {
		t.Fatalf("first Begin returned error: %v", err)
	}
	if !first.Acquired {
		t.Fatal("expected first Begin to acquire slot")
	}

	clock.Advance(time.Minute)

	second, err := store.Begin(context.Background(), "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-b",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       2,
		TTL:           2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("second Begin returned error: %v", err)
	}
	if !second.Acquired {
		t.Fatal("expected Begin to reacquire when previous record expires at now boundary")
	}
	if second.Duplicate {
		t.Fatal("expected reacquire to not be marked duplicate")
	}
}

func TestMemoryStoreCompletePreservesOriginalMetadataAndSetsRetentionTTL(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 3, 26, 11, 0, 0, 0, time.UTC))
	store := idempotency.NewMemoryStoreWithNow(clock.Now)

	_, err := store.Begin(context.Background(), "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       3,
		TTL:           time.Minute,
	})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	clock.Advance(10 * time.Second)

	if err := store.Complete(context.Background(), "msg:123", idempotency.CompleteInput{
		OwnerID:   "worker-b",
		MessageID: "override-me",
		TTL:       5 * time.Minute,
	}); err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	record, err := store.Get(context.Background(), "msg:123")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if record.Status != idempotency.StatusCompleted {
		t.Fatalf("expected status completed, got %q", record.Status)
	}
	if record.OwnerID != "worker-a" {
		t.Fatalf("expected owner metadata to be preserved, got %q", record.OwnerID)
	}
	if record.MessageID != "123" {
		t.Fatalf("expected message metadata to be preserved, got %q", record.MessageID)
	}
	if record.ConsumerGroup != "payments" {
		t.Fatalf("expected consumer group to be preserved, got %q", record.ConsumerGroup)
	}
	if record.Attempt != 3 {
		t.Fatalf("expected attempt to be preserved, got %d", record.Attempt)
	}
	expectedExpiry := clock.Now().Add(5 * time.Minute)
	if !record.ExpiresAt.Equal(expectedExpiry) {
		t.Fatalf("expected completion retention expiry %s, got %s", expectedExpiry, record.ExpiresAt)
	}
}

func TestMemoryStoreFailPreservesOriginalMetadataAndSetsRetentionTTL(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC))
	store := idempotency.NewMemoryStoreWithNow(clock.Now)

	_, err := store.Begin(context.Background(), "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       4,
		TTL:           time.Minute,
	})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	clock.Advance(20 * time.Second)

	if err := store.Fail(context.Background(), "msg:123", idempotency.FailInput{
		OwnerID:   "worker-b",
		MessageID: "override-me",
		TTL:       7 * time.Minute,
	}); err != nil {
		t.Fatalf("Fail returned error: %v", err)
	}

	record, err := store.Get(context.Background(), "msg:123")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if record.Status != idempotency.StatusFailed {
		t.Fatalf("expected status failed, got %q", record.Status)
	}
	if record.OwnerID != "worker-a" {
		t.Fatalf("expected owner metadata to be preserved, got %q", record.OwnerID)
	}
	if record.MessageID != "123" {
		t.Fatalf("expected message metadata to be preserved, got %q", record.MessageID)
	}
	if record.ConsumerGroup != "payments" {
		t.Fatalf("expected consumer group to be preserved, got %q", record.ConsumerGroup)
	}
	if record.Attempt != 4 {
		t.Fatalf("expected attempt to be preserved, got %d", record.Attempt)
	}
	expectedExpiry := clock.Now().Add(7 * time.Minute)
	if !record.ExpiresAt.Equal(expectedExpiry) {
		t.Fatalf("expected failure retention expiry %s, got %s", expectedExpiry, record.ExpiresAt)
	}
}

func TestMemoryStoreRejectsNonPositiveTTL(t *testing.T) {
	store := idempotency.NewMemoryStore()
	ctx := context.Background()

	_, err := store.Begin(ctx, "msg:begin:0", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       1,
		TTL:           0,
	})
	if err == nil {
		t.Fatal("expected Begin with zero ttl to return error")
	}

	_, err = store.Begin(ctx, "msg:begin:-1", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       1,
		TTL:           -time.Second,
	})
	if err == nil {
		t.Fatal("expected Begin with negative ttl to return error")
	}

	if err := store.Complete(ctx, "msg:complete:0", idempotency.CompleteInput{
		OwnerID:   "worker-a",
		MessageID: "123",
		TTL:       0,
	}); err == nil {
		t.Fatal("expected Complete with zero ttl to return error")
	}

	if err := store.Complete(ctx, "msg:complete:-1", idempotency.CompleteInput{
		OwnerID:   "worker-a",
		MessageID: "123",
		TTL:       -time.Second,
	}); err == nil {
		t.Fatal("expected Complete with negative ttl to return error")
	}

	if err := store.Fail(ctx, "msg:fail:0", idempotency.FailInput{
		OwnerID:   "worker-a",
		MessageID: "123",
		TTL:       0,
	}); err == nil {
		t.Fatal("expected Fail with zero ttl to return error")
	}

	if err := store.Fail(ctx, "msg:fail:-1", idempotency.FailInput{
		OwnerID:   "worker-a",
		MessageID: "123",
		TTL:       -time.Second,
	}); err == nil {
		t.Fatal("expected Fail with negative ttl to return error")
	}
}
