package redis

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"lockman/backend"
)

func TestMiniRedisDriverAcquireStrictIssuesPositiveFencingToken(t *testing.T) {
	s := newMiniRedis(t)
	drv := New(goredis.NewClient(&goredis.Options{Addr: s.Addr()}), "")

	strict, ok := any(drv).(backend.StrictDriver)
	if !ok {
		t.Fatal("expected backend.StrictDriver capability")
	}

	acquired, err := strict.AcquireStrict(context.Background(), backend.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     2 * time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict returned error: %v", err)
	}
	if acquired.FencingToken == 0 {
		t.Fatal("expected fencing token > 0")
	}
}

func TestMiniRedisDriverAcquireWithLineageRejectsOverlap(t *testing.T) {
	s := newMiniRedis(t)

	prefix := fmt.Sprintf("lockman:test:%s", t.Name())
	clientA := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	clientB := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = clientA.Close() })
	t.Cleanup(func() { _ = clientB.Close() })

	drvA := New(clientA, prefix)
	drvB := New(clientB, prefix)

	lineageA, ok := any(drvA).(backend.LineageDriver)
	if !ok {
		t.Fatal("expected backend.LineageDriver capability")
	}
	lineageB, ok := any(drvB).(backend.LineageDriver)
	if !ok {
		t.Fatal("expected backend.LineageDriver capability")
	}

	parentReq := backend.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "runtime-parent",
		LeaseTTL:     2 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "parent-lease",
			Kind:    backend.KindParent,
		},
	}
	parentLease, err := lineageA.AcquireWithLineage(context.Background(), parentReq)
	if err != nil {
		t.Fatalf("AcquireWithLineage parent returned error: %v", err)
	}
	t.Cleanup(func() { _ = lineageA.ReleaseWithLineage(context.Background(), parentLease, parentReq.Lineage) })

	_, err = lineageB.AcquireWithLineage(context.Background(), backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "runtime-child",
		LeaseTTL:     2 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "child-lease",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	})
	if !errors.Is(err, backend.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
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
