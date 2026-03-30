package backend

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrInvalidRequest is returned when the backend cannot process the request because
	// it violates the backend contract (for example: missing resource keys).
	ErrInvalidRequest = errors.New("backend: invalid request")

	// ErrLeaseAlreadyHeld is returned when a resource is already leased (or otherwise
	// unavailable) for the requested acquire.
	ErrLeaseAlreadyHeld = errors.New("backend: lease already held")

	// ErrOverlapRejected is returned when a lineage-aware backend rejects an acquire
	// because the requested resource overlaps an active ancestor or descendant lease.
	ErrOverlapRejected = errors.New("backend: overlap rejected")

	// ErrLeaseNotFound is returned when the provided lease cannot be located.
	ErrLeaseNotFound = errors.New("backend: lease not found")

	// ErrLeaseExpired is returned when the provided lease is no longer valid.
	ErrLeaseExpired = errors.New("backend: lease expired")

	// ErrLeaseOwnerMismatch is returned when the lease exists but does not belong to
	// the provided owner identity.
	ErrLeaseOwnerMismatch = errors.New("backend: lease owner mismatch")
)

// AcquireRequest describes the inputs required to obtain a lease for a resource.
type AcquireRequest struct {
	DefinitionID string
	ResourceKeys []string
	OwnerID      string
	LeaseTTL     time.Duration
}

// LeaseRecord represents metadata returned after a successful lease operation.
type LeaseRecord struct {
	DefinitionID string
	ResourceKeys []string
	OwnerID      string
	LeaseTTL     time.Duration
	AcquiredAt   time.Time
	ExpiresAt    time.Time
}

func (l LeaseRecord) IsExpired(now time.Time) bool {
	if l.LeaseTTL <= 0 {
		return true
	}
	return now.After(l.ExpiresAt)
}

// FencedLeaseRecord wraps a lease record with the fencing token issued by strict backends.
type FencedLeaseRecord struct {
	Lease        LeaseRecord
	FencingToken uint64
}

// PresenceRequest encapsulates the inputs required to inspect a resource's current state.
type PresenceRequest struct {
	DefinitionID string
	ResourceKeys []string
}

// PresenceRecord surfaces whether the resource is actively leased and, if so, lease metadata.
type PresenceRecord struct {
	Present      bool
	DefinitionID string
	ResourceKeys []string
	Lease        LeaseRecord
}

// Driver defines the backend contract any lock backend must fulfill.
type Driver interface {
	Acquire(ctx context.Context, req AcquireRequest) (LeaseRecord, error)
	Renew(ctx context.Context, lease LeaseRecord) (LeaseRecord, error)
	Release(ctx context.Context, lease LeaseRecord) error
	CheckPresence(ctx context.Context, req PresenceRequest) (PresenceRecord, error)
	Ping(ctx context.Context) error
}

// StrictAcquireRequest describes the inputs required to obtain a strict-mode lease
// for a single resource key with a fencing token.
type StrictAcquireRequest struct {
	DefinitionID string
	ResourceKey  string
	OwnerID      string
	LeaseTTL     time.Duration
}

// StrictDriver is an optional capability for backends that can issue fencing tokens
// for strict-mode acquire/renew/release operations.
type StrictDriver interface {
	AcquireStrict(ctx context.Context, req StrictAcquireRequest) (FencedLeaseRecord, error)
	RenewStrict(ctx context.Context, lease LeaseRecord, fencingToken uint64) (FencedLeaseRecord, error)
	ReleaseStrict(ctx context.Context, lease LeaseRecord, fencingToken uint64) error
}

// LockKind distinguishes parent and child lineage definitions in backend lineage metadata.
type LockKind string

const (
	KindParent LockKind = "parent"
	KindChild  LockKind = "child"
)

// AncestorKey describes an ancestor lock resource key for lineage-aware operations.
type AncestorKey struct {
	DefinitionID string
	ResourceKey  string
}

// LineageLeaseMeta includes the lineage details backends must persist or return for
// lineage-aware renew/release operations.
type LineageLeaseMeta struct {
	LeaseID      string
	Kind         LockKind
	AncestorKeys []AncestorKey
}

// LineageAcquireRequest describes the inputs required to acquire a lineage-aware lease.
type LineageAcquireRequest struct {
	DefinitionID string
	ResourceKey  string
	OwnerID      string
	LeaseTTL     time.Duration
	Lineage      LineageLeaseMeta
}

// LineageDriver is an optional capability for backends that can execute lineage-aware
// acquire/renew/release operations (for example: parent-child overlap protection).
type LineageDriver interface {
	AcquireWithLineage(ctx context.Context, req LineageAcquireRequest) (LeaseRecord, error)
	RenewWithLineage(ctx context.Context, lease LeaseRecord, lineage LineageLeaseMeta) (LeaseRecord, LineageLeaseMeta, error)
	ReleaseWithLineage(ctx context.Context, lease LeaseRecord, lineage LineageLeaseMeta) error
}
