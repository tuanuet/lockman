// Package idempotency is a compatibility shim for the promoted idempotency contracts.
//
// Deprecated: Import "lockman/idempotency" instead. This shim exists to keep older
// lockkit import paths building during the migration and may be removed in a later
// release.
package idempotency

import promoted "lockman/idempotency"

// Deprecated: use lockman/idempotency.Status.
type Status = promoted.Status

const (
	StatusMissing    = promoted.StatusMissing
	StatusInProgress = promoted.StatusInProgress
	StatusCompleted  = promoted.StatusCompleted
	StatusFailed     = promoted.StatusFailed
)

// Deprecated: use lockman/idempotency.Record.
type Record = promoted.Record

// Deprecated: use lockman/idempotency.BeginInput.
type BeginInput = promoted.BeginInput

// Deprecated: use lockman/idempotency.BeginResult.
type BeginResult = promoted.BeginResult

// Deprecated: use lockman/idempotency.CompleteInput.
type CompleteInput = promoted.CompleteInput

// Deprecated: use lockman/idempotency.FailInput.
type FailInput = promoted.FailInput

// Deprecated: use lockman/idempotency.Store.
type Store = promoted.Store
