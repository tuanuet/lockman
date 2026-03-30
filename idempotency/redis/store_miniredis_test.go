package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman/idempotency"
)

func TestMiniRedisStoreBasicSemantics(t *testing.T) {
	s := newMiniRedis(t)
	client := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	store := NewStore(client, "")
	var _ idempotency.Store = store

	ctx := context.Background()
	key := "msg:123"

	missing, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get missing returned error: %v", err)
	}
	if missing.Status != idempotency.StatusMissing {
		t.Fatalf("expected missing status, got %q", missing.Status)
	}

	first, err := store.Begin(ctx, key, idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       1,
		TTL:           2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if !first.Acquired || first.Duplicate {
		t.Fatalf("expected acquired=true duplicate=false, got %#v", first)
	}
	if first.Record.Status != idempotency.StatusInProgress {
		t.Fatalf("expected in_progress record, got %q", first.Record.Status)
	}

	second, err := store.Begin(ctx, key, idempotency.BeginInput{
		OwnerID:       "worker-b",
		MessageID:     "override-me",
		ConsumerGroup: "payments",
		Attempt:       2,
		TTL:           2 * time.Second,
	})
	if err != nil {
		t.Fatalf("second Begin returned error: %v", err)
	}
	if second.Acquired || !second.Duplicate {
		t.Fatalf("expected acquired=false duplicate=true, got %#v", second)
	}
	if second.Record.OwnerID != "worker-a" || second.Record.MessageID != "123" || second.Record.Attempt != 1 {
		t.Fatalf("expected original metadata preserved on duplicate begin, got %#v", second.Record)
	}

	if err := store.Complete(ctx, key, idempotency.CompleteInput{
		OwnerID:   "worker-b",
		MessageID: "override-me",
		TTL:       5 * time.Second,
	}); err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	record, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after Complete returned error: %v", err)
	}
	if record.Status != idempotency.StatusCompleted {
		t.Fatalf("expected completed status, got %q", record.Status)
	}
	if record.OwnerID != "worker-a" || record.MessageID != "123" || record.ConsumerGroup != "payments" || record.Attempt != 1 {
		t.Fatalf("expected original metadata preserved after completion, got %#v", record)
	}
}

func newMiniRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run failed: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}
