package testkit

import (
	"context"
	"sync"
	"time"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	lockerrors "lockman/lockkit/errors"
)

// MemoryDriver is a naive single-resource driver useful for tests and local builds.
type MemoryDriver struct {
	mu     sync.Mutex
	leases map[string]drivers.LeaseRecord

	// lineageLeases stores lineage metadata by lease id so descendant membership can be pruned reliably.
	lineageLeases map[string]lineageLeaseState

	// descendantsByAncestor maps a stable ancestor key (definition + resource key) to active descendant lease ids.
	descendantsByAncestor map[string]map[string]time.Time
}

type lineageLeaseState struct {
	lease    drivers.LeaseRecord
	lineage  drivers.LineageLeaseMeta
	expireAt time.Time
}

// NewMemoryDriver returns a ready-to-use in-memory driver.
func NewMemoryDriver() *MemoryDriver {
	return &MemoryDriver{
		leases:                make(map[string]drivers.LeaseRecord),
		lineageLeases:         make(map[string]lineageLeaseState),
		descendantsByAncestor: make(map[string]map[string]time.Time),
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

	m.pruneExpired(now)

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

	m.pruneExpired(now)

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

	m.pruneExpired(time.Now())

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

	m.pruneExpired(now)

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

func (m *MemoryDriver) AcquireWithLineage(ctx context.Context, req drivers.LineageAcquireRequest) (drivers.LeaseRecord, error) {
	if req.ResourceKey == "" {
		return drivers.LeaseRecord{}, drivers.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return drivers.LeaseRecord{}, drivers.ErrInvalidRequest
	}
	if req.Lineage.LeaseID == "" {
		return drivers.LeaseRecord{}, drivers.ErrInvalidRequest
	}

	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)
	if err := m.rejectLineageConflict(now, req); err != nil {
		return drivers.LeaseRecord{}, err
	}

	lease := drivers.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{req.ResourceKey},
		OwnerID:      req.OwnerID,
		LeaseTTL:     req.LeaseTTL,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(req.LeaseTTL),
	}

	meta := drivers.LineageLeaseMeta{
		LeaseID:      req.Lineage.LeaseID,
		Kind:         req.Lineage.Kind,
		AncestorKeys: cloneAncestorKeys(req.Lineage.AncestorKeys),
	}

	m.storeLeaseAndMembership(lease, meta)
	return lease, nil
}

func (m *MemoryDriver) RenewWithLineage(ctx context.Context, lease drivers.LeaseRecord, lineage drivers.LineageLeaseMeta) (drivers.LeaseRecord, drivers.LineageLeaseMeta, error) {
	if len(lease.ResourceKeys) != 1 {
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, drivers.ErrInvalidRequest
	}
	if lineage.LeaseID == "" {
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, drivers.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	existing, ok := m.leases[key]
	if !ok {
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, drivers.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, drivers.ErrLeaseOwnerMismatch
	}
	if existing.IsExpired(now) {
		delete(m.leases, key)
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, drivers.ErrLeaseExpired
	}

	ttl := lease.LeaseTTL
	if ttl <= 0 {
		ttl = existing.LeaseTTL
	}
	existing.LeaseTTL = ttl
	existing.AcquiredAt = now
	existing.ExpiresAt = now.Add(ttl)
	m.leases[key] = existing

	if state, ok := m.lineageLeases[lineage.LeaseID]; ok {
		state.lease = existing
		state.expireAt = existing.ExpiresAt
		m.lineageLeases[lineage.LeaseID] = state
	}

	// Extend descendant membership TTL.
	for _, ancestor := range lineage.AncestorKeys {
		ancestorKey := formatAncestorKey(ancestor)
		members := m.descendantsByAncestor[ancestorKey]
		if members == nil {
			continue
		}
		if _, ok := members[lineage.LeaseID]; ok {
			members[lineage.LeaseID] = existing.ExpiresAt
		}
		if len(members) == 0 {
			delete(m.descendantsByAncestor, ancestorKey)
		}
	}

	return existing, drivers.LineageLeaseMeta{
		LeaseID:      lineage.LeaseID,
		Kind:         lineage.Kind,
		AncestorKeys: cloneAncestorKeys(lineage.AncestorKeys),
	}, nil
}

func (m *MemoryDriver) ReleaseWithLineage(ctx context.Context, lease drivers.LeaseRecord, lineage drivers.LineageLeaseMeta) error {
	if len(lease.ResourceKeys) != 1 {
		return drivers.ErrInvalidRequest
	}
	if lineage.LeaseID == "" {
		return drivers.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	existing, ok := m.leases[key]
	if !ok {
		return drivers.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return drivers.ErrLeaseOwnerMismatch
	}

	delete(m.leases, key)
	delete(m.lineageLeases, lineage.LeaseID)
	for _, ancestor := range lineage.AncestorKeys {
		ancestorKey := formatAncestorKey(ancestor)
		members := m.descendantsByAncestor[ancestorKey]
		if members == nil {
			continue
		}
		delete(members, lineage.LeaseID)
		if len(members) == 0 {
			delete(m.descendantsByAncestor, ancestorKey)
		}
	}

	return nil
}

// Ping always succeeds for the in-memory driver.
func (m *MemoryDriver) Ping(ctx context.Context) error {
	return nil
}

func (m *MemoryDriver) rejectLineageConflict(now time.Time, req drivers.LineageAcquireRequest) error {
	// Exact-key conflicts are handled as plain contention.
	if existing, ok := m.leases[req.ResourceKey]; ok && !existing.IsExpired(now) {
		return drivers.ErrLeaseAlreadyHeld
	}

	switch req.Lineage.Kind {
	case definitions.KindParent:
		ancestorKey := formatAncestorKey(drivers.AncestorKey{
			DefinitionID: req.DefinitionID,
			ResourceKey:  req.ResourceKey,
		})
		for _, expireAt := range m.descendantsByAncestor[ancestorKey] {
			if now.Before(expireAt) {
				return lockerrors.ErrOverlapRejected
			}
		}
	case definitions.KindChild:
		for _, ancestor := range req.Lineage.AncestorKeys {
			if existing, ok := m.leases[ancestor.ResourceKey]; ok && !existing.IsExpired(now) {
				return lockerrors.ErrOverlapRejected
			}
		}
	}

	return nil
}

func (m *MemoryDriver) storeLeaseAndMembership(lease drivers.LeaseRecord, lineage drivers.LineageLeaseMeta) {
	key := lease.ResourceKeys[0]
	m.leases[key] = lease
	m.lineageLeases[lineage.LeaseID] = lineageLeaseState{
		lease:    lease,
		lineage:  lineage,
		expireAt: lease.ExpiresAt,
	}

	for _, ancestor := range lineage.AncestorKeys {
		ancestorKey := formatAncestorKey(ancestor)
		members := m.descendantsByAncestor[ancestorKey]
		if members == nil {
			members = make(map[string]time.Time)
			m.descendantsByAncestor[ancestorKey] = members
		}
		members[lineage.LeaseID] = lease.ExpiresAt
	}
}

func (m *MemoryDriver) pruneExpired(now time.Time) {
	for key, lease := range m.leases {
		if lease.IsExpired(now) {
			delete(m.leases, key)
		}
	}

	for leaseID, state := range m.lineageLeases {
		if now.After(state.expireAt) || state.lease.IsExpired(now) {
			delete(m.lineageLeases, leaseID)
			for _, ancestor := range state.lineage.AncestorKeys {
				ancestorKey := formatAncestorKey(ancestor)
				members := m.descendantsByAncestor[ancestorKey]
				if members == nil {
					continue
				}
				delete(members, leaseID)
				if len(members) == 0 {
					delete(m.descendantsByAncestor, ancestorKey)
				}
			}
		}
	}
}

func formatAncestorKey(key drivers.AncestorKey) string {
	// Stable join; resource keys may contain ":" so use a separator unlikely to appear.
	return key.DefinitionID + "\x00" + key.ResourceKey
}

func cloneAncestorKeys(input []drivers.AncestorKey) []drivers.AncestorKey {
	if len(input) == 0 {
		return nil
	}
	out := make([]drivers.AncestorKey, len(input))
	copy(out, input)
	return out
}
