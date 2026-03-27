package redis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	lockerrors "lockman/lockkit/errors"
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

func TestDriverRenewExtendsTTL(t *testing.T) {
	env := newRedisTestEnv(t)
	ctx := context.Background()

	lease, err := env.driver.Acquire(ctx, drivers.AcquireRequest{
		DefinitionID: "order.lock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "worker-a",
		LeaseTTL:     1500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	renewed, err := env.driver.Renew(ctx, drivers.LeaseRecord{
		DefinitionID: lease.DefinitionID,
		ResourceKeys: lease.ResourceKeys,
		OwnerID:      lease.OwnerID,
		LeaseTTL:     4 * time.Second,
	})
	if err != nil {
		t.Fatalf("Renew returned error: %v", err)
	}

	if renewed.LeaseTTL < 4*time.Second {
		t.Fatalf("expected renewed TTL >= 4s, got %s", renewed.LeaseTTL)
	}
	if !renewed.ExpiresAt.After(lease.ExpiresAt) {
		t.Fatalf("expected renewed expiry to extend lease window: old=%s new=%s", lease.ExpiresAt, renewed.ExpiresAt)
	}
}

func TestDriverRenewRejectsWrongOwner(t *testing.T) {
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

	_, err = driver.Renew(ctx, drivers.LeaseRecord{
		DefinitionID: lease.DefinitionID,
		ResourceKeys: lease.ResourceKeys,
		OwnerID:      "worker-b",
		LeaseTTL:     time.Minute,
	})
	if !errors.Is(err, drivers.ErrLeaseOwnerMismatch) {
		t.Fatalf("expected owner mismatch, got %v", err)
	}
}

func TestDriverRenewRejectsNonPositiveTTL(t *testing.T) {
	env := newRedisTestEnv(t)
	ctx := context.Background()

	lease, err := env.driver.Acquire(ctx, drivers.AcquireRequest{
		DefinitionID: "order.lock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "worker-a",
		LeaseTTL:     time.Minute,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	hook := &commandCaptureHook{}
	env.client.AddHook(hook)
	hook.reset()

	_, err = env.driver.Renew(ctx, drivers.LeaseRecord{
		DefinitionID: lease.DefinitionID,
		ResourceKeys: lease.ResourceKeys,
		OwnerID:      lease.OwnerID,
		LeaseTTL:     0,
	})
	if !errors.Is(err, drivers.ErrInvalidRequest) {
		t.Fatalf("expected invalid request for non-positive renew ttl, got %v", err)
	}
	if hook.contains("eval") || hook.contains("evalsha") {
		t.Fatalf("expected renew ttl validation to fail before script execution, got commands %v", hook.commandList())
	}
}

func TestDriverPingIsConnectivityOnlyAndScriptsStillWork(t *testing.T) {
	env := newRedisTestEnv(t)
	ctx := context.Background()

	hook := &commandCaptureHook{}
	env.client.AddHook(hook)
	hook.reset()

	if err := env.driver.Ping(ctx); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}
	if hook.contains("script") || hook.contains("script_load") || hook.contains("scriptload") {
		t.Fatalf("expected Ping to avoid script loading, got commands %v", hook.commandList())
	}
	if !hook.contains("ping") {
		t.Fatalf("expected Ping command to be issued, got commands %v", hook.commandList())
	}

	lease, err := env.driver.Acquire(ctx, drivers.AcquireRequest{
		DefinitionID: "order.lock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "worker-a",
		LeaseTTL:     time.Minute,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}
	if _, err := env.driver.Renew(ctx, drivers.LeaseRecord{
		DefinitionID: lease.DefinitionID,
		ResourceKeys: lease.ResourceKeys,
		OwnerID:      lease.OwnerID,
		LeaseTTL:     time.Minute,
	}); err != nil {
		t.Fatalf("Renew after Ping returned error: %v", err)
	}
	if err := env.driver.Release(ctx, lease); err != nil {
		t.Fatalf("Release after Ping returned error: %v", err)
	}
}

func TestDriverAcquireWithLineageRejectsChildWhileParentHeldAcrossClients(t *testing.T) {
	clientA := newRedisClientForTest(t)
	clientB := newRedisClientForTest(t)
	prefix := newRedisTestPrefix(t)
	driverA := NewDriver(clientA, prefix)
	driverB := NewDriver(clientB, prefix)

	parentReq := newParentAcquireRequest("parent-lease")
	parentLease, err := driverA.AcquireWithLineage(context.Background(), parentReq)
	if err != nil {
		t.Fatalf("parent acquire failed: %v", err)
	}
	defer func() {
		_ = driverA.ReleaseWithLineage(context.Background(), parentLease, parentReq.Lineage)
	}()

	_, err = driverB.AcquireWithLineage(context.Background(), newChildAcquireRequest("child-lease", "line-1", 2*time.Second))
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
	}
}

func TestDriverRenewWithLineageExtendsDescendantMembershipTTL(t *testing.T) {
	clientA := newRedisClientForTest(t)
	clientB := newRedisClientForTest(t)
	prefix := newRedisTestPrefix(t)
	driverA := NewDriver(clientA, prefix)
	driverB := NewDriver(clientB, prefix)

	childReq := newChildAcquireRequest("child-lease", "line-1", 200*time.Millisecond)
	childLease, err := driverA.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	childMeta := childReq.Lineage
	defer func() {
		_ = driverA.ReleaseWithLineage(context.Background(), childLease, childMeta)
	}()

	time.Sleep(100 * time.Millisecond)

	childLease.LeaseTTL = 200 * time.Millisecond
	childLease, childMeta, err = driverA.RenewWithLineage(context.Background(), childLease, childMeta)
	if err != nil {
		t.Fatalf("child renew failed: %v", err)
	}

	time.Sleep(130 * time.Millisecond)

	_, err = driverB.AcquireWithLineage(context.Background(), newParentAcquireRequest("parent-lease"))
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection after renew, got %v", err)
	}
}

func TestDriverReleaseWithLineageRemovesOnlyReleasedMembership(t *testing.T) {
	clientA := newRedisClientForTest(t)
	clientB := newRedisClientForTest(t)
	prefix := newRedisTestPrefix(t)
	driverA := NewDriver(clientA, prefix)
	driverB := NewDriver(clientB, prefix)

	childOneReq := newChildAcquireRequest("child-one", "line-1", 2*time.Second)
	childOne, err := driverA.AcquireWithLineage(context.Background(), childOneReq)
	if err != nil {
		t.Fatalf("child one acquire failed: %v", err)
	}
	childTwoReq := newChildAcquireRequest("child-two", "line-2", 2*time.Second)
	childTwo, err := driverA.AcquireWithLineage(context.Background(), childTwoReq)
	if err != nil {
		t.Fatalf("child two acquire failed: %v", err)
	}

	if err := driverA.ReleaseWithLineage(context.Background(), childOne, childOneReq.Lineage); err != nil {
		t.Fatalf("release child one failed: %v", err)
	}

	_, err = driverB.AcquireWithLineage(context.Background(), newParentAcquireRequest("parent-after-first-release"))
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected parent to stay blocked by child two, got %v", err)
	}

	if err := driverA.ReleaseWithLineage(context.Background(), childTwo, childTwoReq.Lineage); err != nil {
		t.Fatalf("release child two failed: %v", err)
	}

	parentReq := newParentAcquireRequest("parent-after-second-release")
	parentLease, err := driverB.AcquireWithLineage(context.Background(), parentReq)
	if err != nil {
		t.Fatalf("expected parent acquire to succeed after both children release, got %v", err)
	}
	_ = driverB.ReleaseWithLineage(context.Background(), parentLease, parentReq.Lineage)
}

func TestDriverExpiredChildNoLongerBlocksParentAcquire(t *testing.T) {
	driver := newRedisDriverForTest(t)

	childReq := newChildAcquireRequest("expiring-child", "line-1", 120*time.Millisecond)
	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}

	time.Sleep(180 * time.Millisecond)

	parentReq := newParentAcquireRequest("parent-after-expiry")
	parentLease, err := driver.AcquireWithLineage(context.Background(), parentReq)
	if err != nil {
		t.Fatalf("expected parent acquire after expiry, got %v", err)
	}
	_ = driver.ReleaseWithLineage(context.Background(), parentLease, parentReq.Lineage)
	_ = driver.ReleaseWithLineage(context.Background(), childLease, childReq.Lineage)
}

func TestDriverRenewWithLineageFailsWhenAncestorMembershipMissing(t *testing.T) {
	env := newRedisTestEnv(t)

	childReq := newChildAcquireRequest("child-lease", "line-1", 500*time.Millisecond)
	childLease, err := env.driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}

	ancestor := childReq.Lineage.AncestorKeys[0]
	if err := env.client.Del(context.Background(), env.driver.buildLineageKey(ancestor.DefinitionID, ancestor.ResourceKey)).Err(); err != nil {
		t.Fatalf("delete ancestor lineage key failed: %v", err)
	}

	_, _, err = env.driver.RenewWithLineage(context.Background(), childLease, childReq.Lineage)
	if !errors.Is(err, drivers.ErrLeaseExpired) {
		t.Fatalf("expected renew failure when ancestor membership is missing, got %v", err)
	}
}

func newRedisDriverForTest(t *testing.T) *Driver {
	t.Helper()
	return newRedisTestEnv(t).driver
}

type redisTestEnv struct {
	driver *Driver
	client *goredis.Client
}

func newRedisTestEnv(t *testing.T) redisTestEnv {
	t.Helper()

	client := newRedisClientForTest(t)
	prefix := newRedisTestPrefix(t)
	return redisTestEnv{
		driver: NewDriver(client, prefix),
		client: client,
	}
}

func newRedisClientForTest(t *testing.T) *goredis.Client {
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

	return client
}

func newRedisTestPrefix(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("lockman:test:%s:%d", strings.ToLower(strings.ReplaceAll(t.Name(), "/", ":")), time.Now().UnixNano())
}

func newParentAcquireRequest(leaseID string) drivers.LineageAcquireRequest {
	return drivers.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "runtime-parent",
		LeaseTTL:     2 * time.Second,
		Lineage: drivers.LineageLeaseMeta{
			LeaseID: leaseID,
			Kind:    definitions.KindParent,
		},
	}
}

func newChildAcquireRequest(leaseID, itemID string, ttl time.Duration) drivers.LineageAcquireRequest {
	return drivers.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:" + itemID,
		OwnerID:      "runtime-child",
		LeaseTTL:     ttl,
		Lineage: drivers.LineageLeaseMeta{
			LeaseID: leaseID,
			Kind:    definitions.KindChild,
			AncestorKeys: []drivers.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}
}

type commandCaptureHook struct {
	mu   sync.Mutex
	seen []string
}

func (h *commandCaptureHook) DialHook(next goredis.DialHook) goredis.DialHook {
	return next
}

func (h *commandCaptureHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		h.mu.Lock()
		h.seen = append(h.seen, strings.ToLower(cmd.Name()))
		h.mu.Unlock()
		return next(ctx, cmd)
	}
}

func (h *commandCaptureHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		h.mu.Lock()
		for _, cmd := range cmds {
			h.seen = append(h.seen, strings.ToLower(cmd.Name()))
		}
		h.mu.Unlock()
		return next(ctx, cmds)
	}
}

func (h *commandCaptureHook) reset() {
	h.mu.Lock()
	h.seen = nil
	h.mu.Unlock()
}

func (h *commandCaptureHook) contains(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Contains(h.seen, strings.ToLower(name))
}

func (h *commandCaptureHook) commandList() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.seen...)
}
