package definitions

import "time"

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

type RuntimeOverrides struct {
	WaitTimeout *time.Duration
	MaxRetries  *int
}

type SyncLockRequest struct {
	DefinitionID string
	KeyInput     map[string]string
	Ownership    OwnershipMeta
	Overrides    *RuntimeOverrides
}

type PresenceCheckRequest struct {
	DefinitionID string
	KeyInput     map[string]string
	Ownership    OwnershipMeta
}

type LeaseContext struct {
	DefinitionID  string
	ResourceKey   string
	Ownership     OwnershipMeta
	LeaseTTL      time.Duration
	LeaseDeadline time.Time
}

type PresenceState int

const (
	PresenceHeld PresenceState = iota
	PresenceNotHeld
	PresenceUnknown
)

type PresenceStatus struct {
	State         PresenceState
	Mode          LockMode
	OwnerID       string
	LeaseDeadline time.Time
}
