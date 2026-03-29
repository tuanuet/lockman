package idempotency

import promoted "lockman/idempotency"

type Status = promoted.Status

const (
	StatusMissing    = promoted.StatusMissing
	StatusInProgress = promoted.StatusInProgress
	StatusCompleted  = promoted.StatusCompleted
	StatusFailed     = promoted.StatusFailed
)

type Record = promoted.Record

type BeginInput = promoted.BeginInput
type BeginResult = promoted.BeginResult

type CompleteInput = promoted.CompleteInput
type FailInput = promoted.FailInput

type Store = promoted.Store

