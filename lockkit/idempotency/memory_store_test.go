package idempotency_test

import (
	"context"
	"testing"
	"time"

	"lockman/lockkit/idempotency"
)

func TestMemoryStoreBeginRejectsSecondActiveClaim(t *testing.T) {
	store := idempotency.NewMemoryStore()

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
}
