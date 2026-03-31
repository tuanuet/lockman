package inspect

import (
	"time"

	"github.com/tuanuet/lockman/observe"
)

// RuntimeLockInfo describes an actively held lock.
type RuntimeLockInfo struct {
	DefinitionID string    `json:"definition_id"`
	ResourceID   string    `json:"resource_id"`
	OwnerID      string    `json:"owner_id"`
	AcquiredAt   time.Time `json:"acquired_at"`
}

// WorkerClaimInfo describes a pending acquire attempt.
type WorkerClaimInfo struct {
	DefinitionID string    `json:"definition_id"`
	ResourceID   string    `json:"resource_id"`
	OwnerID      string    `json:"owner_id"`
	ClaimedAt    time.Time `json:"claimed_at"`
}

// RenewalInfo tracks the latest successful renewal for a lock.
type RenewalInfo struct {
	DefinitionID string    `json:"definition_id"`
	ResourceID   string    `json:"resource_id"`
	OwnerID      string    `json:"owner_id"`
	LastRenewed  time.Time `json:"last_renewed"`
}

// ShutdownInfo tracks whether shutdown has started and completed.
type ShutdownInfo struct {
	Started   bool `json:"started"`
	Completed bool `json:"completed"`
}

// PipelineState holds dispatcher-level observability counters.
type PipelineState struct {
	DropPolicy           string `json:"drop_policy,omitempty"`
	BufferSize           int    `json:"buffer_size,omitempty"`
	DroppedCount         int64  `json:"dropped_count"`
	SinkFailureCount     int64  `json:"sink_failure_count"`
	ExporterFailureCount int64  `json:"exporter_failure_count"`
}

// Snapshot is the full point-in-time inspect view.
type Snapshot struct {
	RuntimeLocks []RuntimeLockInfo `json:"runtime_locks"`
	WorkerClaims []WorkerClaimInfo `json:"worker_claims"`
	Renewals     []RenewalInfo     `json:"renewals"`
	Shutdown     ShutdownInfo      `json:"shutdown"`
	Pipeline     PipelineState     `json:"pipeline"`
}

// QueryOptions filters the stored event history.
type QueryOptions struct {
	DefinitionID string
	ResourceID   string
	OwnerID      string
	Kind         observe.EventKind
	Since        time.Time
	Until        time.Time
}
