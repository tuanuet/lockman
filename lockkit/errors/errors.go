package errors

import stdErrors "errors"

var (
	// ErrLockBusy is returned when another client holds the lock.
	ErrLockBusy = stdErrors.New("lock busy")

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
	ErrInvariantRejected = stdErrors.New("invariant rejected")

	// ErrWorkerShuttingDown indicates worker runtime is shutting down.
	ErrWorkerShuttingDown = stdErrors.New("worker shutting down")
)
