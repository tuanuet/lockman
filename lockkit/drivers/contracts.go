package drivers

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidRequest     = errors.New("drivers: invalid request")
	ErrLeaseAlreadyHeld   = errors.New("drivers: lease already held")
	ErrLeaseNotFound      = errors.New("drivers: lease not found")
	ErrLeaseExpired       = errors.New("drivers: lease expired")
	ErrLeaseOwnerMismatch = errors.New("drivers: lease owner mismatch")
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

// PresenceRequest encapsulates the inputs required to inspect a resource's current state.
type PresenceRequest struct {
	DefinitionID string
	ResourceKeys []string
}

// PresenceRecord surfaces whether the resource is actively leased and, if so, by whom.
type PresenceRecord struct {
	Present      bool
	DefinitionID string
	ResourceKeys []string
	Lease        LeaseRecord
}

// Driver defines the backend contract any lock driver must fulfill.
type Driver interface {
	Acquire(ctx context.Context, req AcquireRequest) (LeaseRecord, error)
	Renew(ctx context.Context, lease LeaseRecord) (LeaseRecord, error)
	Release(ctx context.Context, lease LeaseRecord) error
	CheckPresence(ctx context.Context, req PresenceRequest) (PresenceRecord, error)
	Ping(ctx context.Context) error
}
