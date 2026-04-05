package memory

import (
	"context"
	"sync"
	"time"

	"github.com/tuanuet/lockman/backend"
)

// MemoryDriver is a naive single-resource driver useful for tests and local builds.
type MemoryDriver struct {
	mu     sync.Mutex
	leases map[string]backend.LeaseRecord

	// strictCounters tracks fencing tokens by strict boundary (definition + resource key).
	strictCounters map[string]uint64
	// strictLeases tracks active strict lease metadata by strict boundary.
	strictLeases map[string]strictLeaseState

	// lineageLeases stores lineage metadata by lease id so descendant membership can be pruned reliably.
	lineageLeases map[string]lineageLeaseState

	// descendantsByAncestor maps a stable ancestor key (definition + resource key) to active descendant lease ids.
	descendantsByAncestor map[string]map[string]time.Time
}

type lineageLeaseState struct {
	lease    backend.LeaseRecord
	lineage  backend.LineageLeaseMeta
	expireAt time.Time
}

type strictLeaseState struct {
	lease        backend.LeaseRecord
	fencingToken uint64
}

// NewMemoryDriver returns a ready-to-use in-memory driver.
func NewMemoryDriver() *MemoryDriver {
	return &MemoryDriver{
		leases:                make(map[string]backend.LeaseRecord),
		strictCounters:        make(map[string]uint64),
		strictLeases:          make(map[string]strictLeaseState),
		lineageLeases:         make(map[string]lineageLeaseState),
		descendantsByAncestor: make(map[string]map[string]time.Time),
	}
}

// Acquire attempts to claim a single resource. It rejects already held leases.
func (m *MemoryDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	if len(req.ResourceKeys) != 1 {
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	}

	key := req.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	if existing, ok := m.leases[key]; ok {
		if !existing.IsExpired(now) {
			return backend.LeaseRecord{}, backend.ErrLeaseAlreadyHeld
		}
		delete(m.leases, key)
	}

	lease := backend.LeaseRecord{
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
func (m *MemoryDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	if len(lease.ResourceKeys) != 1 {
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	existing, ok := m.leases[key]
	if !ok {
		return backend.LeaseRecord{}, backend.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return backend.LeaseRecord{}, backend.ErrLeaseOwnerMismatch
	}
	if existing.IsExpired(now) {
		delete(m.leases, key)
		return backend.LeaseRecord{}, backend.ErrLeaseExpired
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
func (m *MemoryDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	if len(lease.ResourceKeys) != 1 {
		return backend.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(time.Now())

	existing, ok := m.leases[key]
	if !ok {
		return backend.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return backend.ErrLeaseOwnerMismatch
	}

	delete(m.leases, key)
	return nil
}

// AcquireStrict attempts to claim a single resource and returns a fencing token.
func (m *MemoryDriver) AcquireStrict(ctx context.Context, req backend.StrictAcquireRequest) (backend.FencedLeaseRecord, error) {
	if req.ResourceKey == "" {
		return backend.FencedLeaseRecord{}, backend.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return backend.FencedLeaseRecord{}, backend.ErrInvalidRequest
	}

	key := req.ResourceKey
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	if existing, ok := m.leases[key]; ok {
		if !existing.IsExpired(now) {
			return backend.FencedLeaseRecord{}, backend.ErrLeaseAlreadyHeld
		}
		delete(m.leases, key)
		delete(m.strictLeases, strictBoundaryKey(existing.DefinitionID, key))
	}

	lease := backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{req.ResourceKey},
		OwnerID:      req.OwnerID,
		LeaseTTL:     req.LeaseTTL,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(req.LeaseTTL),
	}

	boundary := strictBoundaryKey(req.DefinitionID, req.ResourceKey)
	nextToken := m.strictCounters[boundary] + 1
	m.strictCounters[boundary] = nextToken

	m.leases[key] = lease
	m.strictLeases[boundary] = strictLeaseState{
		lease:        lease,
		fencingToken: nextToken,
	}

	return backend.FencedLeaseRecord{
		Lease:        lease,
		FencingToken: nextToken,
	}, nil
}

// RenewStrict refreshes an existing strict lease while preserving its fencing token.
func (m *MemoryDriver) RenewStrict(ctx context.Context, lease backend.LeaseRecord, fencingToken uint64) (backend.FencedLeaseRecord, error) {
	if len(lease.ResourceKeys) != 1 {
		return backend.FencedLeaseRecord{}, backend.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]
	boundary := strictBoundaryKey(lease.DefinitionID, key)
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	existing, ok := m.leases[key]
	if !ok {
		return backend.FencedLeaseRecord{}, backend.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return backend.FencedLeaseRecord{}, backend.ErrLeaseOwnerMismatch
	}
	if existing.IsExpired(now) {
		delete(m.leases, key)
		delete(m.strictLeases, boundary)
		return backend.FencedLeaseRecord{}, backend.ErrLeaseExpired
	}
	if existing.DefinitionID != lease.DefinitionID {
		return backend.FencedLeaseRecord{}, backend.ErrLeaseOwnerMismatch
	}

	strictState, ok := m.strictLeases[boundary]
	if !ok {
		return backend.FencedLeaseRecord{}, backend.ErrLeaseNotFound
	}
	if strictState.lease.OwnerID != lease.OwnerID || strictState.fencingToken != fencingToken {
		return backend.FencedLeaseRecord{}, backend.ErrLeaseOwnerMismatch
	}

	ttl := lease.LeaseTTL
	if ttl <= 0 {
		ttl = existing.LeaseTTL
	}
	existing.LeaseTTL = ttl
	existing.AcquiredAt = now
	existing.ExpiresAt = now.Add(ttl)
	m.leases[key] = existing

	strictState.lease = existing
	m.strictLeases[boundary] = strictState

	return backend.FencedLeaseRecord{
		Lease:        existing,
		FencingToken: strictState.fencingToken,
	}, nil
}

// ReleaseStrict removes a strict lease after owner and fencing token validation.
func (m *MemoryDriver) ReleaseStrict(ctx context.Context, lease backend.LeaseRecord, fencingToken uint64) error {
	if len(lease.ResourceKeys) != 1 {
		return backend.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]
	boundary := strictBoundaryKey(lease.DefinitionID, key)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(time.Now())

	existing, ok := m.leases[key]
	if !ok {
		return backend.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return backend.ErrLeaseOwnerMismatch
	}
	if existing.DefinitionID != lease.DefinitionID {
		return backend.ErrLeaseOwnerMismatch
	}

	strictState, ok := m.strictLeases[boundary]
	if !ok {
		return backend.ErrLeaseNotFound
	}
	if strictState.lease.OwnerID != lease.OwnerID || strictState.fencingToken != fencingToken {
		return backend.ErrLeaseOwnerMismatch
	}

	delete(m.leases, key)
	delete(m.strictLeases, boundary)
	return nil
}

// CheckPresence reports whether the resource is currently held.
func (m *MemoryDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	if len(req.ResourceKeys) != 1 {
		return backend.PresenceRecord{}, backend.ErrInvalidRequest
	}

	key := req.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	record := backend.PresenceRecord{
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

func (m *MemoryDriver) AcquireWithLineage(ctx context.Context, req backend.LineageAcquireRequest) (backend.LeaseRecord, error) {
	if req.ResourceKey == "" {
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	}
	if req.Lineage.LeaseID == "" {
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	}

	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)
	if err := m.rejectLineageConflict(now, req); err != nil {
		return backend.LeaseRecord{}, err
	}

	lease := backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{req.ResourceKey},
		OwnerID:      req.OwnerID,
		LeaseTTL:     req.LeaseTTL,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(req.LeaseTTL),
	}

	meta := backend.LineageLeaseMeta{
		LeaseID:      req.Lineage.LeaseID,
		Kind:         req.Lineage.Kind,
		AncestorKeys: cloneAncestorKeys(req.Lineage.AncestorKeys),
	}

	m.storeLeaseAndMembership(lease, meta)
	return lease, nil
}

func (m *MemoryDriver) RenewWithLineage(ctx context.Context, lease backend.LeaseRecord, lineage backend.LineageLeaseMeta) (backend.LeaseRecord, backend.LineageLeaseMeta, error) {
	if len(lease.ResourceKeys) != 1 {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrInvalidRequest
	}
	if lineage.LeaseID == "" {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	existing, ok := m.leases[key]
	if !ok {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseOwnerMismatch
	}
	if existing.IsExpired(now) {
		delete(m.leases, key)
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseExpired
	}

	state, ok := m.lineageLeases[lineage.LeaseID]
	if !ok {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseExpired
	}
	for _, ancestor := range lineage.AncestorKeys {
		ancestorKey := formatAncestorKey(ancestor)
		members := m.descendantsByAncestor[ancestorKey]
		if members == nil {
			return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseExpired
		}
		if _, ok := members[lineage.LeaseID]; !ok {
			return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseExpired
		}
	}

	ttl := lease.LeaseTTL
	if ttl <= 0 {
		ttl = existing.LeaseTTL
	}
	existing.LeaseTTL = ttl
	existing.AcquiredAt = now
	existing.ExpiresAt = now.Add(ttl)
	m.leases[key] = existing

	state.lease = existing
	state.expireAt = existing.ExpiresAt
	m.lineageLeases[lineage.LeaseID] = state

	for _, ancestor := range lineage.AncestorKeys {
		ancestorKey := formatAncestorKey(ancestor)
		members := m.descendantsByAncestor[ancestorKey]
		if members == nil {
			continue
		}
		if _, ok := members[lineage.LeaseID]; ok {
			members[lineage.LeaseID] = existing.ExpiresAt
		}
	}

	return existing, backend.LineageLeaseMeta{
		LeaseID:      lineage.LeaseID,
		Kind:         lineage.Kind,
		AncestorKeys: cloneAncestorKeys(lineage.AncestorKeys),
	}, nil
}

func (m *MemoryDriver) ReleaseWithLineage(ctx context.Context, lease backend.LeaseRecord, lineage backend.LineageLeaseMeta) error {
	if len(lease.ResourceKeys) != 1 {
		return backend.ErrInvalidRequest
	}
	if lineage.LeaseID == "" {
		return backend.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	existing, ok := m.leases[key]
	if !ok {
		return backend.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return backend.ErrLeaseOwnerMismatch
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

func (m *MemoryDriver) rejectLineageConflict(now time.Time, req backend.LineageAcquireRequest) error {
	if existing, ok := m.leases[req.ResourceKey]; ok && !existing.IsExpired(now) {
		return backend.ErrLeaseAlreadyHeld
	}

	switch req.Lineage.Kind {
	case backend.KindParent:
		ancestorKey := formatAncestorKey(backend.AncestorKey{
			DefinitionID: req.DefinitionID,
			ResourceKey:  req.ResourceKey,
		})
		for _, expireAt := range m.descendantsByAncestor[ancestorKey] {
			if now.Before(expireAt) {
				return backend.ErrOverlapRejected
			}
		}
	case backend.KindChild:
		for _, ancestor := range req.Lineage.AncestorKeys {
			if existing, ok := m.leases[ancestor.ResourceKey]; ok && !existing.IsExpired(now) {
				return backend.ErrOverlapRejected
			}
		}
	}

	return nil
}

func (m *MemoryDriver) storeLeaseAndMembership(lease backend.LeaseRecord, lineage backend.LineageLeaseMeta) {
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
			delete(m.strictLeases, strictBoundaryKey(lease.DefinitionID, key))
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

func strictBoundaryKey(definitionID, resourceKey string) string {
	return definitionID + "\x00" + resourceKey
}

func formatAncestorKey(key backend.AncestorKey) string {
	return key.DefinitionID + "\x00" + key.ResourceKey
}

func cloneAncestorKeys(input []backend.AncestorKey) []backend.AncestorKey {
	if len(input) == 0 {
		return nil
	}
	out := make([]backend.AncestorKey, len(input))
	copy(out, input)
	return out
}

var (
	_ backend.Driver        = (*MemoryDriver)(nil)
	_ backend.StrictDriver  = (*MemoryDriver)(nil)
	_ backend.LineageDriver = (*MemoryDriver)(nil)
)
