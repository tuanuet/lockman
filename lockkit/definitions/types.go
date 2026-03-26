package definitions

import "time"

type LockMode string

const (
	ModeStandard LockMode = "standard"
	ModeStrict   LockMode = "strict"
)

type LockKind string

const (
	KindParent LockKind = "parent"
	KindChild  LockKind = "child"
)

type ExecutionKind string

const (
	ExecutionSync  ExecutionKind = "sync"
	ExecutionAsync ExecutionKind = "async"
	ExecutionBoth  ExecutionKind = "both"
)

type BackendFailurePolicy string

const (
	BackendFailClosed     BackendFailurePolicy = "fail_closed"
	BackendBestEffortOpen BackendFailurePolicy = "best_effort_open"
)

type RetryPolicy struct {
	MaxRetries int
}

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
	KeyBuilder           KeyBuilder
	Tags                 map[string]string
}
