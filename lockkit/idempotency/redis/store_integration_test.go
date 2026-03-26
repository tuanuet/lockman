package redis

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"lockman/lockkit/idempotency"
)

func TestStoreBeginRejectsSecondActiveClaim(t *testing.T) {
	store := newRedisStoreForTest(t)
	ctx := context.Background()
	inProgressTTL := time.Minute

	first, err := store.Begin(ctx, "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       1,
		TTL:           inProgressTTL,
	})
	if err != nil {
		t.Fatalf("first Begin returned error: %v", err)
	}
	if !first.Acquired || first.Duplicate {
		t.Fatalf("expected first begin acquire=true duplicate=false, got %#v", first)
	}
	assertKeyPTTLAtMostAndPositive(t, store, "msg:123", inProgressTTL)

	second, err := store.Begin(ctx, "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-b",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       2,
		TTL:           time.Minute,
	})
	if err != nil {
		t.Fatalf("second Begin returned error: %v", err)
	}
	if second.Acquired || !second.Duplicate {
		t.Fatalf("expected second begin acquire=false duplicate=true, got %#v", second)
	}
	if second.Record.Status != idempotency.StatusInProgress {
		t.Fatalf("expected existing record status in_progress, got %q", second.Record.Status)
	}
}

func TestStoreCompletePersistsTerminalRecord(t *testing.T) {
	store := newRedisStoreForTest(t)
	ctx := context.Background()

	_, err := store.Begin(ctx, "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       1,
		TTL:           time.Minute,
	})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	if err := store.Complete(ctx, "msg:123", idempotency.CompleteInput{
		OwnerID:   "worker-a",
		MessageID: "123",
		TTL:       24 * time.Hour,
	}); err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	record, err := store.Get(ctx, "msg:123")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if record.Status != idempotency.StatusCompleted {
		t.Fatalf("expected completed record, got %q", record.Status)
	}
}

func TestStoreCompletePreservesOriginalMetadataAndUsesTerminalTTL(t *testing.T) {
	store := newRedisStoreForTest(t)
	ctx := context.Background()
	inProgressTTL := 20 * time.Second
	terminalTTL := 5 * time.Minute

	_, err := store.Begin(ctx, "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       3,
		TTL:           inProgressTTL,
	})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	assertKeyPTTLAtMostAndPositive(t, store, "msg:123", inProgressTTL)

	if err := store.Complete(ctx, "msg:123", idempotency.CompleteInput{
		OwnerID:   "worker-b",
		MessageID: "override-me",
		TTL:       terminalTTL,
	}); err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	assertKeyPTTLAtMostAndPositive(t, store, "msg:123", terminalTTL)

	record, err := store.Get(ctx, "msg:123")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if record.Status != idempotency.StatusCompleted {
		t.Fatalf("expected completed status, got %q", record.Status)
	}
	if record.OwnerID != "worker-a" || record.MessageID != "123" || record.ConsumerGroup != "payments" || record.Attempt != 3 {
		t.Fatalf("expected original metadata preserved, got %#v", record)
	}
	if !record.ExpiresAt.After(time.Now().Add(4 * time.Minute)) {
		t.Fatalf("expected terminal retention ttl to be applied, got expires_at=%s", record.ExpiresAt)
	}
}

func TestStoreFailPreservesOriginalMetadataAndUsesTerminalTTL(t *testing.T) {
	store := newRedisStoreForTest(t)
	ctx := context.Background()
	inProgressTTL := 20 * time.Second
	terminalTTL := 7 * time.Minute

	_, err := store.Begin(ctx, "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       4,
		TTL:           inProgressTTL,
	})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	assertKeyPTTLAtMostAndPositive(t, store, "msg:123", inProgressTTL)

	if err := store.Fail(ctx, "msg:123", idempotency.FailInput{
		OwnerID:   "worker-b",
		MessageID: "override-me",
		TTL:       terminalTTL,
	}); err != nil {
		t.Fatalf("Fail returned error: %v", err)
	}
	assertKeyPTTLAtMostAndPositive(t, store, "msg:123", terminalTTL)

	record, err := store.Get(ctx, "msg:123")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if record.Status != idempotency.StatusFailed {
		t.Fatalf("expected failed status, got %q", record.Status)
	}
	if record.OwnerID != "worker-a" || record.MessageID != "123" || record.ConsumerGroup != "payments" || record.Attempt != 4 {
		t.Fatalf("expected original metadata preserved, got %#v", record)
	}
	if !record.ExpiresAt.After(time.Now().Add(6 * time.Minute)) {
		t.Fatalf("expected terminal retention ttl to be applied, got expires_at=%s", record.ExpiresAt)
	}
}

func newRedisStoreForTest(t *testing.T) *Store {
	t.Helper()

	redisURL := strings.TrimSpace(os.Getenv("LOCKMAN_REDIS_URL"))
	if redisURL == "" {
		t.Skip("LOCKMAN_REDIS_URL is not set")
	}

	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("ParseURL returned error: %v", err)
	}

	client := goredis.NewClient(opts)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})

	prefix := fmt.Sprintf("lockman:test:idempotency:%s:%d", strings.ToLower(strings.ReplaceAll(t.Name(), "/", ":")), time.Now().UnixNano())
	return NewStore(client, prefix)
}

func assertKeyPTTLAtMostAndPositive(t *testing.T, store *Store, key string, max time.Duration) {
	t.Helper()

	ttl, err := store.client.PTTL(context.Background(), store.buildKey(key)).Result()
	if err != nil {
		t.Fatalf("PTTL returned error: %v", err)
	}
	if ttl <= 0 {
		t.Fatalf("expected key pttl > 0, got %s", ttl)
	}
	if ttl > max {
		t.Fatalf("expected key pttl <= %s, got %s", max, ttl)
	}
}
