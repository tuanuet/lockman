package definitions

import "time"

// OwnershipMeta carries caller metadata for lock operations.
type OwnershipMeta struct {
	ServiceName   string
	InstanceID    string
	HandlerName   string
	OwnerID       string
	RequestID     string
	MessageID     string
	Attempt       int
	ConsumerGroup string
}

// RuntimeOverrides allows a caller to override timeout or retry behavior at runtime.
type RuntimeOverrides struct {
	WaitTimeout *time.Duration
	MaxRetries  *int
}

// SyncLockRequest is the payload for synchronous acquire attempts.
type SyncLockRequest struct {
	DefinitionID string
	KeyInput     map[string]string
	Ownership    OwnershipMeta
	Overrides    *RuntimeOverrides
}

// MessageClaimRequest is the payload for asynchronous claimed execution.
type MessageClaimRequest struct {
	DefinitionID   string
	KeyInput       map[string]string
	Ownership      OwnershipMeta
	IdempotencyKey string
	Overrides      *RuntimeOverrides
}

// CompositeLockRequest is the payload for synchronous composite acquire attempts.
type CompositeLockRequest struct {
	DefinitionID string
	// MemberInputs must follow the CompositeDefinition.Members order.
	MemberInputs []map[string]string
	Ownership    OwnershipMeta
	Overrides    *RuntimeOverrides
}

// CompositeClaimRequest is the payload for asynchronous composite claimed execution.
type CompositeClaimRequest struct {
	DefinitionID   string
	MemberInputs   []map[string]string
	Ownership      OwnershipMeta
	IdempotencyKey string
	Overrides      *RuntimeOverrides
}

// PresenceCheckRequest asks whether a lock key is currently held.
type PresenceCheckRequest struct {
	DefinitionID string
	KeyInput     map[string]string
	Ownership    OwnershipMeta
}

// LeaseContext tracks ownership and TTL information for a granted lease.
// ResourceKey is used for single-resource execution; ResourceKeys is populated for composite execution.
type LeaseContext struct {
	DefinitionID  string
	ResourceKey   string
	ResourceKeys  []string
	Ownership     OwnershipMeta
	FencingToken  uint64
	LeaseTTL      time.Duration
	LeaseDeadline time.Time
}

// ClaimContext tracks ownership and TTL information for a claimed execution.
// ResourceKey is used for single-resource execution; ResourceKeys is populated for composite execution.
type ClaimContext struct {
	DefinitionID   string
	ResourceKey    string
	ResourceKeys   []string
	Ownership      OwnershipMeta
	FencingToken   uint64
	LeaseTTL       time.Duration
	LeaseDeadline  time.Time
	IdempotencyKey string
}

// PresenceState describes whether a lock is held, not held, or in an unknown state with PresenceUnknown as the zero value.
type PresenceState int

const (
	PresenceUnknown PresenceState = iota
	PresenceHeld
	PresenceNotHeld
)

// PresenceStatus reports current ownership state back to callers.
type PresenceStatus struct {
	State         PresenceState
	Mode          LockMode
	OwnerID       string
	LeaseDeadline time.Time
}
