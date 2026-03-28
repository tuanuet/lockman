package lockman

import "errors"

var (
	ErrBusy                = errors.New("lockman: resource busy")
	ErrTimeout             = errors.New("lockman: acquire timed out")
	ErrDuplicate           = errors.New("lockman: duplicate message ignored")
	ErrUseCaseNotFound     = errors.New("lockman: use case not registered")
	ErrRegistryMismatch    = errors.New("lockman: use case does not belong to this registry")
	ErrRegistryRequired    = errors.New("lockman: registry is required")
	ErrIdempotencyRequired = errors.New("lockman: idempotency backend is required for this claim use case")
)
