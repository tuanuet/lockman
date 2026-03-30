package backend_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
)

func TestSentinelErrorIdentitySurvivesErrorsIs(t *testing.T) {
	wrapped := fmt.Errorf("wrapped: %w", backend.ErrLeaseAlreadyHeld)
	if !errors.Is(wrapped, backend.ErrLeaseAlreadyHeld) {
		t.Fatalf("expected errors.Is to match backend.ErrLeaseAlreadyHeld")
	}

	joined := errors.Join(errors.New("other"), backend.ErrLeaseNotFound)
	if !errors.Is(joined, backend.ErrLeaseNotFound) {
		t.Fatalf("expected errors.Is to match backend.ErrLeaseNotFound through errors.Join")
	}

	overlap := fmt.Errorf("wrapped: %w", backend.ErrOverlapRejected)
	if !errors.Is(overlap, backend.ErrOverlapRejected) {
		t.Fatalf("expected errors.Is to match backend.ErrOverlapRejected")
	}
	if got, want := backend.ErrOverlapRejected.Error(), "overlap rejected"; got != want {
		t.Fatalf("ErrOverlapRejected message = %q, want %q", got, want)
	}
}

func TestLineageKindTypeIsBackendScoped(t *testing.T) {
	metaType := reflect.TypeOf(backend.LineageLeaseMeta{})
	field, ok := metaType.FieldByName("Kind")
	if !ok {
		t.Fatal("expected backend.LineageLeaseMeta.Kind field to exist")
	}

	if got, want := field.Type.PkgPath(), "github.com/tuanuet/lockman/backend"; got != want {
		t.Fatalf("LineageLeaseMeta.Kind PkgPath = %q, want %q", got, want)
	}
	if got, want := field.Type.Name(), "LockKind"; got != want {
		t.Fatalf("LineageLeaseMeta.Kind Name = %q, want %q", got, want)
	}
}

func TestCapabilityDetectionStillWorks(t *testing.T) {
	var base backend.Driver = baseDriver{}
	if _, ok := any(base).(backend.StrictDriver); ok {
		t.Fatalf("expected base driver to not satisfy backend.StrictDriver")
	}
	if _, ok := any(base).(backend.LineageDriver); ok {
		t.Fatalf("expected base driver to not satisfy backend.LineageDriver")
	}

	var strict backend.Driver = strictDriver{}
	if _, ok := any(strict).(backend.StrictDriver); !ok {
		t.Fatalf("expected strict driver to satisfy backend.StrictDriver via type assertion")
	}
	if _, ok := any(strict).(backend.LineageDriver); ok {
		t.Fatalf("expected strict driver to not satisfy backend.LineageDriver")
	}

	var lineage backend.Driver = lineageDriver{}
	if _, ok := any(lineage).(backend.LineageDriver); !ok {
		t.Fatalf("expected lineage driver to satisfy backend.LineageDriver via type assertion")
	}
	if _, ok := any(lineage).(backend.StrictDriver); ok {
		t.Fatalf("expected lineage driver to not satisfy backend.StrictDriver")
	}
}

type baseDriver struct{}

func (baseDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	return backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: req.ResourceKeys,
		OwnerID:      req.OwnerID,
		LeaseTTL:     req.LeaseTTL,
	}, nil
}

func (baseDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	lease.ExpiresAt = time.Now().Add(lease.LeaseTTL)
	return lease, nil
}

func (baseDriver) Release(ctx context.Context, lease backend.LeaseRecord) error { return nil }

func (baseDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return backend.PresenceRecord{Present: false, DefinitionID: req.DefinitionID, ResourceKeys: req.ResourceKeys}, nil
}

func (baseDriver) Ping(ctx context.Context) error { return nil }

type strictDriver struct{ baseDriver }

func (strictDriver) AcquireStrict(ctx context.Context, req backend.StrictAcquireRequest) (backend.FencedLeaseRecord, error) {
	return backend.FencedLeaseRecord{
		Lease: backend.LeaseRecord{
			DefinitionID: req.DefinitionID,
			ResourceKeys: []string{req.ResourceKey},
			OwnerID:      req.OwnerID,
			LeaseTTL:     req.LeaseTTL,
		},
		FencingToken: 1,
	}, nil
}

func (strictDriver) RenewStrict(ctx context.Context, lease backend.LeaseRecord, fencingToken uint64) (backend.FencedLeaseRecord, error) {
	return backend.FencedLeaseRecord{Lease: lease, FencingToken: fencingToken}, nil
}

func (strictDriver) ReleaseStrict(ctx context.Context, lease backend.LeaseRecord, fencingToken uint64) error {
	return nil
}

type lineageDriver struct{ baseDriver }

func (lineageDriver) AcquireWithLineage(ctx context.Context, req backend.LineageAcquireRequest) (backend.LeaseRecord, error) {
	return backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{req.ResourceKey},
		OwnerID:      req.OwnerID,
		LeaseTTL:     req.LeaseTTL,
	}, nil
}

func (lineageDriver) RenewWithLineage(ctx context.Context, lease backend.LeaseRecord, lineage backend.LineageLeaseMeta) (backend.LeaseRecord, backend.LineageLeaseMeta, error) {
	return lease, lineage, nil
}

func (lineageDriver) ReleaseWithLineage(ctx context.Context, lease backend.LeaseRecord, lineage backend.LineageLeaseMeta) error {
	return nil
}
