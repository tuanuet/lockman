package definitions

import "time"

// LockMode controls strictness guarantees enforced by a lock definition.
type LockMode string

const (
	ModeStandard LockMode = "standard"
	ModeStrict   LockMode = "strict"
)

// LockKind distinguishes parent and child definitions.
type LockKind string

const (
	KindParent LockKind = "parent"
	KindChild  LockKind = "child"
)

// ExecutionKind describes whether a lock run is synchronous, asynchronous, or either.
type ExecutionKind string

const (
	ExecutionSync  ExecutionKind = "sync"
	ExecutionAsync ExecutionKind = "async"
	ExecutionBoth  ExecutionKind = "both"
)

// OverlapPolicy controls how child lock overlap is handled.
type OverlapPolicy string

const (
	OverlapReject   OverlapPolicy = "reject"
	OverlapEscalate OverlapPolicy = "escalate"
)

// OrderingPolicy defines ordering behavior for composite definitions.
type OrderingPolicy string

const (
	OrderingCanonical OrderingPolicy = "canonical"
)

// AcquirePolicy defines acquire behavior for composite definitions.
type AcquirePolicy string

const (
	AcquireAllOrNothing AcquirePolicy = "all_or_nothing"
)

// EscalationPolicy defines escalation behavior for composite definitions.
type EscalationPolicy string

const (
	EscalationReject EscalationPolicy = "reject"
)

// ModeResolution defines how member lock modes are resolved in composite execution.
type ModeResolution string

const (
	ModeResolutionHomogeneous ModeResolution = "homogeneous"
)

// BackendFailurePolicy describes how the system reacts to downstream failures.
type BackendFailurePolicy string

const (
	BackendFailClosed     BackendFailurePolicy = "fail_closed"
	BackendBestEffortOpen BackendFailurePolicy = "best_effort_open"
)

// RetryPolicy defines how many times the system retries an acquire.
type RetryPolicy struct {
	MaxRetries int
}

// CompositeDefinition defines an ordered lock set treated as one logical operation.
type CompositeDefinition struct {
	ID               string
	Members          []string
	OrderingPolicy   OrderingPolicy
	AcquirePolicy    AcquirePolicy
	EscalationPolicy EscalationPolicy
	ModeResolution   ModeResolution
	MaxMemberCount   int
	ExecutionKind    ExecutionKind
}

// LockDefinition captures the runtime constraints and metadata for a lock.
type LockDefinition struct {
	ID                   string
	Kind                 LockKind
	Resource             string
	Mode                 LockMode
	ExecutionKind        ExecutionKind
	LeaseTTL             time.Duration
	WaitTimeout          time.Duration
	RetryPolicy          RetryPolicy
	BackendFailurePolicy BackendFailurePolicy
	FencingRequired      bool
	IdempotencyRequired  bool
	CheckOnlyAllowed     bool
	Rank                 int
	ParentRef            string
	OverlapPolicy        OverlapPolicy
	KeyBuilder           KeyBuilder
	Tags                 map[string]string // Tags must remain immutable once the definition is registered.
}
