package testkit

import (
	"context"
	"sync"
	"time"

	"lockman/lockkit/drivers"
)

// MemoryDriver is a naive single-resource driver useful for tests and local builds.
type MemoryDriver struct {
	mu     sync.Mutex
	leases map[string]drivers.LeaseRecord
}

// NewMemoryDriver returns a ready-to-use in-memory driver.
func NewMemoryDriver() *MemoryDriver {
	return &MemoryDriver{
		leases: make(map[string]drivers.LeaseRecord),
	}
}

// Acquire attempts to claim a single resource. It rejects already held leases.
func (m *MemoryDriver) Acquire(ctx context.Context, req drivers.AcquireRequest) (drivers.LeaseRecord, error) {
	if len(req.ResourceKeys) != 1 {
		return drivers.LeaseRecord{}, drivers.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return drivers.LeaseRecord{}, drivers.ErrInvalidRequest
	}

	key := req.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.leases[key]; ok {
		if !existing.IsExpired(now) {
			return drivers.LeaseRecord{}, drivers.ErrLeaseAlreadyHeld
		}
		delete(m.leases, key)
	}

	lease := drivers.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: append([]string(nil), req.ResourceKeys...),
		OwnerID:      req.OwnerID,
		LeaseTTL:     req.LeaseTTL,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(req.LeaseTTL),
	}
	m.leases[key] = lease
	return lease, nil
}

// Renew refreshes an existing lease while the owner still holds it.
func (m *MemoryDriver) Renew(ctx context.Context, lease drivers.LeaseRecord) (drivers.LeaseRecord, error) {
	if len(lease.ResourceKeys) != 1 {
		return drivers.LeaseRecord{}, drivers.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.leases[key]
	if !ok {
		return drivers.LeaseRecord{}, drivers.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return drivers.LeaseRecord{}, drivers.ErrLeaseOwnerMismatch
	}
	if existing.IsExpired(now) {
		delete(m.leases, key)
		return drivers.LeaseRecord{}, drivers.ErrLeaseExpired
	}

	ttl := lease.LeaseTTL
	if ttl <= 0 {
		ttl = existing.LeaseTTL
	}
	existing.LeaseTTL = ttl
	existing.AcquiredAt = now
	existing.ExpiresAt = now.Add(ttl)
	m.leases[key] = existing
	return existing, nil
}

// Release removes the lease so another owner can claim the resource.
func (m *MemoryDriver) Release(ctx context.Context, lease drivers.LeaseRecord) error {
	if len(lease.ResourceKeys) != 1 {
		return drivers.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.leases[key]
	if !ok {
		return drivers.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return drivers.ErrLeaseOwnerMismatch
	}

	delete(m.leases, key)
	return nil
}

// CheckPresence reports whether the resource is currently held.
func (m *MemoryDriver) CheckPresence(ctx context.Context, req drivers.PresenceRequest) (drivers.PresenceRecord, error) {
	if len(req.ResourceKeys) != 1 {
		return drivers.PresenceRecord{}, drivers.ErrInvalidRequest
	}

	key := req.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	record := drivers.PresenceRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{key},
	}

	if lease, ok := m.leases[key]; ok {
		if lease.IsExpired(now) {
			delete(m.leases, key)
			return record, nil
		}
		record.Lease = lease
		record.Present = true
		record.DefinitionID = lease.DefinitionID
		record.ResourceKeys = append([]string(nil), lease.ResourceKeys...)
		return record, nil
	}

	return record, nil
}

// Ping always succeeds for the in-memory driver.
func (m *MemoryDriver) Ping(ctx context.Context) error {
	return nil
}
