package redis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"lockman/lockkit/drivers"
)

func TestDriverReleaseRejectsWrongOwner(t *testing.T) {
	driver := newRedisDriverForTest(t)
	ctx := context.Background()

	lease, err := driver.Acquire(ctx, drivers.AcquireRequest{
		DefinitionID: "order.lock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "worker-a",
		LeaseTTL:     time.Minute,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	err = driver.Release(ctx, drivers.LeaseRecord{
		DefinitionID: lease.DefinitionID,
		ResourceKeys: lease.ResourceKeys,
		OwnerID:      "worker-b",
		LeaseTTL:     lease.LeaseTTL,
	})
	if !errors.Is(err, drivers.ErrLeaseOwnerMismatch) {
		t.Fatalf("expected owner mismatch, got %v", err)
	}
}

func TestDriverCheckPresenceReturnsOwnerAndExpiry(t *testing.T) {
	driver := newRedisDriverForTest(t)
	ctx := context.Background()

	lease, err := driver.Acquire(ctx, drivers.AcquireRequest{
		DefinitionID: "order.lock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "worker-a",
		LeaseTTL:     time.Minute,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	record, err := driver.CheckPresence(ctx, drivers.PresenceRequest{
		DefinitionID: "order.lock",
		ResourceKeys: []string{"order:123"},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}
	if !record.Present || record.Lease.OwnerID != lease.OwnerID || record.Lease.ExpiresAt.IsZero() {
		t.Fatalf("expected owner and expiry metadata, got %#v", record)
	}
}

func newRedisDriverForTest(t *testing.T) *Driver {
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
	t.Cleanup(func() { _ = client.Close() })

	prefix := fmt.Sprintf("lockman:test:%s:%d", strings.ToLower(strings.ReplaceAll(t.Name(), "/", ":")), time.Now().UnixNano())
	return NewDriver(client, prefix)
}
