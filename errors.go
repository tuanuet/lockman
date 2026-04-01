package lockman

import "errors"

var (
	ErrBusy                      = errors.New("lockman: resource busy")
	ErrTimeout                   = errors.New("lockman: acquire timed out")
	ErrDuplicate                 = errors.New("lockman: duplicate message ignored")
	ErrOverlapRejected           = errors.New("lockman: overlap rejected")
	ErrShuttingDown              = errors.New("lockman: shutting down")
	ErrLeaseLost                 = errors.New("lockman: lease lost before completion")
	ErrInvariantRejected         = errors.New("lockman: execution invariant rejected")
	ErrUseCaseNotFound           = errors.New("lockman: use case not registered")
	ErrRegistryMismatch          = errors.New("lockman: use case does not belong to this registry")
	ErrRegistryRequired          = errors.New("lockman: registry is required")
	ErrIdentityRequired          = errors.New("lockman: owner identity is required")
	ErrBackendRequired           = errors.New("lockman: backend is required")
	ErrHoldTokenInvalid          = errors.New("lockman: hold token is malformed or unrecognized")
	ErrHoldExpired               = errors.New("lockman: hold lease has expired")
	ErrBackendCapabilityRequired = errors.New("lockman: backend lacks required capability")
	ErrIdempotencyRequired       = errors.New("lockman: idempotency backend is required for this claim use case")
)
