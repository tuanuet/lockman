package lockman

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
)

type batchOrderInput struct {
	OrderID string
}

func TestRunMultipleAcquiresAllKeys(t *testing.T) {
	orderDef := DefineLock(
		"order",
		BindResourceID("order", func(in batchOrderInput) string { return in.OrderID }),
	)
	batchUC := DefineRunOn("batch_process", orderDef, TTL(5*time.Second))

	reg := NewRegistry()
	if err := reg.Register(batchUC); err != nil {
		t.Fatal(err)
	}

	drv := &trackingMultipleBackend{}
	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(drv),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	req1, _ := batchUC.With(batchOrderInput{OrderID: "1"})
	req2, _ := batchUC.With(batchOrderInput{OrderID: "2"})
	req3, _ := batchUC.With(batchOrderInput{OrderID: "3"})

	var gotKeys []string
	err = client.RunMultiple(context.Background(), func(ctx context.Context, lease Lease) error {
		gotKeys = append([]string(nil), lease.ResourceKeys...)
		return nil
	}, []RunRequest{req1, req2, req3})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotKeys) != 3 {
		t.Fatalf("expected 3 keys, got %d: %v", len(gotKeys), gotKeys)
	}
}

func TestRunMultipleAllOrNothing(t *testing.T) {
	orderDef := DefineLock(
		"order",
		BindResourceID("order", func(in batchOrderInput) string { return in.OrderID }),
	)
	batchUC := DefineRunOn("batch_process", orderDef, TTL(5*time.Second))

	reg := NewRegistry()
	if err := reg.Register(batchUC); err != nil {
		t.Fatal(err)
	}

	drv := &trackingMultipleBackend{failOnKey: "order:2"}
	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(drv),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	req1, _ := batchUC.With(batchOrderInput{OrderID: "1"})
	req2, _ := batchUC.With(batchOrderInput{OrderID: "2"})
	req3, _ := batchUC.With(batchOrderInput{OrderID: "3"})

	called := false
	err = client.RunMultiple(context.Background(), func(ctx context.Context, lease Lease) error {
		called = true
		return nil
	}, []RunRequest{req1, req2, req3})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy, got: %v", err)
	}
	if called {
		t.Fatal("callback should not be called on failure")
	}
}

func TestRunMultipleRejectsEmptyRequests(t *testing.T) {
	orderDef := DefineLock(
		"order",
		BindResourceID("order", func(in batchOrderInput) string { return in.OrderID }),
	)
	batchUC := DefineRunOn("batch_process", orderDef)

	reg := NewRegistry()
	if err := reg.Register(batchUC); err != nil {
		t.Fatal(err)
	}

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(&trackingMultipleBackend{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	err = client.RunMultiple(context.Background(), func(ctx context.Context, lease Lease) error {
		return nil
	}, []RunRequest{})

	if err == nil {
		t.Fatal("expected error for empty requests")
	}
}

func TestRunMultipleRejectsDuplicateKeys(t *testing.T) {
	orderDef := DefineLock(
		"order",
		BindResourceID("order", func(in batchOrderInput) string { return in.OrderID }),
	)
	batchUC := DefineRunOn("batch_process", orderDef)

	reg := NewRegistry()
	if err := reg.Register(batchUC); err != nil {
		t.Fatal(err)
	}

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(&trackingMultipleBackend{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	req1, _ := batchUC.With(batchOrderInput{OrderID: "1"})
	req2, _ := batchUC.With(batchOrderInput{OrderID: "1"})

	err = client.RunMultiple(context.Background(), func(ctx context.Context, lease Lease) error {
		return nil
	}, []RunRequest{req1, req2})

	if err == nil {
		t.Fatal("expected error for duplicate keys")
	}
}

func TestRunMultipleRejectsNilCallback(t *testing.T) {
	orderDef := DefineLock(
		"order",
		BindResourceID("order", func(in batchOrderInput) string { return in.OrderID }),
	)
	batchUC := DefineRunOn("batch_process", orderDef)

	reg := NewRegistry()
	if err := reg.Register(batchUC); err != nil {
		t.Fatal(err)
	}

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(&trackingMultipleBackend{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	req1, _ := batchUC.With(batchOrderInput{OrderID: "1"})

	err = client.RunMultiple(context.Background(), nil, []RunRequest{req1})

	if err == nil {
		t.Fatal("expected error for nil callback")
	}
}

func TestHoldMultipleAcquiresAllKeys(t *testing.T) {
	slotDef := DefineLock(
		"slot",
		BindResourceID("slot", func(in batchOrderInput) string { return in.OrderID }),
	)
	holdUC := DefineHoldOn("hold_slots", slotDef, TTL(5*time.Second))

	reg := NewRegistry()
	if err := reg.Register(holdUC); err != nil {
		t.Fatal(err)
	}

	drv := &trackingMultipleBackend{}
	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(drv),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	req1, _ := holdUC.With(batchOrderInput{OrderID: "1"})
	req2, _ := holdUC.With(batchOrderInput{OrderID: "2"})
	req3, _ := holdUC.With(batchOrderInput{OrderID: "3"})

	handle, err := client.HoldMultiple(context.Background(), []HoldRequest{req1, req2, req3})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handle.Token() == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestHoldMultipleForfeitReleasesAllKeys(t *testing.T) {
	slotDef := DefineLock(
		"slot",
		BindResourceID("slot", func(in batchOrderInput) string { return in.OrderID }),
	)
	holdUC := DefineHoldOn("hold_slots", slotDef, TTL(5*time.Second))

	reg := NewRegistry()
	if err := reg.Register(holdUC); err != nil {
		t.Fatal(err)
	}

	drv := &trackingMultipleBackend{}
	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(drv),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	req1, _ := holdUC.With(batchOrderInput{OrderID: "1"})
	req2, _ := holdUC.With(batchOrderInput{OrderID: "2"})

	handle, err := client.HoldMultiple(context.Background(), []HoldRequest{req1, req2})
	if err != nil {
		t.Fatal(err)
	}

	err = client.Forfeit(context.Background(), holdUC.ForfeitWith(handle.Token()))
	if err != nil {
		t.Fatalf("unexpected forfeit error: %v", err)
	}
}

func TestHoldMultipleRejectsEmptyRequests(t *testing.T) {
	slotDef := DefineLock(
		"slot",
		BindResourceID("slot", func(in batchOrderInput) string { return in.OrderID }),
	)
	holdUC := DefineHoldOn("hold_slots", slotDef)

	reg := NewRegistry()
	if err := reg.Register(holdUC); err != nil {
		t.Fatal(err)
	}

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(&trackingMultipleBackend{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	_, err = client.HoldMultiple(context.Background(), []HoldRequest{})

	if err == nil {
		t.Fatal("expected error for empty requests")
	}
}

// trackingMultipleBackend tracks acquire/release calls for test assertions
type trackingMultipleBackend struct {
	failOnKey   string
	acquireKeys []string
	releaseKeys []string
}

func (d *trackingMultipleBackend) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	key := req.ResourceKeys[0]
	d.acquireKeys = append(d.acquireKeys, key)
	if key == d.failOnKey {
		return backend.LeaseRecord{}, backend.ErrLeaseAlreadyHeld
	}
	return backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: req.ResourceKeys,
		OwnerID:      req.OwnerID,
		AcquiredAt:   time.Now(),
		ExpiresAt:    time.Now().Add(req.LeaseTTL),
		LeaseTTL:     req.LeaseTTL,
	}, nil
}

func (d *trackingMultipleBackend) Renew(ctx context.Context, rec backend.LeaseRecord) (backend.LeaseRecord, error) {
	return rec, nil
}

func (d *trackingMultipleBackend) Release(ctx context.Context, rec backend.LeaseRecord) error {
	d.releaseKeys = append(d.releaseKeys, rec.ResourceKeys...)
	return nil
}

func (d *trackingMultipleBackend) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return backend.PresenceRecord{Present: false}, nil
}

func (d *trackingMultipleBackend) Ping(ctx context.Context) error {
	return nil
}
