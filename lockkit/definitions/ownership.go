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

// PresenceCheckRequest asks whether a lock key is currently held.
type PresenceCheckRequest struct {
	DefinitionID string
	KeyInput     map[string]string
	Ownership    OwnershipMeta
}

// LeaseContext tracks ownership and TTL information for a granted lease.
type LeaseContext struct {
	DefinitionID  string
	ResourceKey   string
	Ownership     OwnershipMeta
	LeaseTTL      time.Duration
	LeaseDeadline time.Time
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
