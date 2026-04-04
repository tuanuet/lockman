package errors

import (
	stdErrors "errors"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/guard"
)

var (
	// ErrLockBusy is returned when another client holds the lock.
	ErrLockBusy = stdErrors.New("lock busy")

	// ErrOverlapRejected is returned when lineage overlap is rejected.
	// This must remain distinct from ErrLockBusy so workers can normalize retry behavior explicitly.
	ErrOverlapRejected = backend.ErrOverlapRejected

	// ErrLockAcquireTimeout is returned when lock acquisition exceeds the configured timeout.
	ErrLockAcquireTimeout = stdErrors.New("lock acquire timeout")

	// ErrLeaseLost signals that the lock lease was reclaimed before completion.
	ErrLeaseLost = stdErrors.New("lease lost")

	// ErrRegistryViolation indicates the lock registry invariant was breached.
	ErrRegistryViolation = stdErrors.New("registry violation")

	// ErrPolicyViolation indicates a policy prevented lock acquisition.
	ErrPolicyViolation = stdErrors.New("policy violation")

	// ErrReentrantAcquire is returned when the same client reacquires the lock without releasing.
	ErrReentrantAcquire = stdErrors.New("reentrant acquire")

	// ErrDuplicateIgnored indicates duplicate processing was safely ignored.
	ErrDuplicateIgnored = stdErrors.New("duplicate ignored")

	// ErrInvariantRejected indicates runtime invariant checks rejected execution.
	ErrInvariantRejected = guard.ErrInvariantRejected

	// ErrWorkerShuttingDown indicates worker runtime is shutting down.
	ErrWorkerShuttingDown = stdErrors.New("worker shutting down")

	// ErrPreconditionFailed indicates a runtime precondition was not met.
	ErrPreconditionFailed = stdErrors.New("precondition failed")
)
