package sdk

import "errors"

var (
	errIdempotencyCapabilityRequired = errors.New("lockman/sdk: idempotency store is required")
	errStrictCapabilityRequired      = errors.New("lockman/sdk: strict backend support is required")
	errLineageCapabilityRequired     = errors.New("lockman/sdk: lineage backend support is required")
)

var (
	// ErrIdempotencyCapabilityRequired is returned when idempotent claim use cases are configured without a store.
	ErrIdempotencyCapabilityRequired = errIdempotencyCapabilityRequired
	// ErrStrictCapabilityRequired is returned when strict use cases are configured without strict backend support.
	ErrStrictCapabilityRequired = errStrictCapabilityRequired
	// ErrLineageCapabilityRequired is returned when lineage use cases are configured without lineage backend support.
	ErrLineageCapabilityRequired = errLineageCapabilityRequired
)

type backendCapabilities struct {
	hasIdempotencyStore bool
	hasStrictBackend    bool
	hasLineageBackend   bool
}

// BackendCapabilities describes backend support visible to package lockman.
type BackendCapabilities struct {
	HasIdempotencyStore bool
	HasStrictBackend    bool
	HasLineageBackend   bool
}

func validateCapabilities(useCases []useCase, capabilities backendCapabilities) error {
	for _, uc := range useCases {
		if uc.requirements.requiresIdempotency && !capabilities.hasIdempotencyStore {
			return errIdempotencyCapabilityRequired
		}
		if uc.requirements.requiresStrict && !capabilities.hasStrictBackend {
			return errStrictCapabilityRequired
		}
		if uc.requirements.requiresLineage && !capabilities.hasLineageBackend {
			return errLineageCapabilityRequired
		}
	}
	return nil
}

// ValidateCapabilities checks whether all normalized use cases can run with the provided backend capabilities.
func ValidateCapabilities(useCases []UseCase, capabilities BackendCapabilities) error {
	return validateCapabilities(internalUseCases(useCases), backendCapabilities{
		hasIdempotencyStore: capabilities.HasIdempotencyStore,
		hasStrictBackend:    capabilities.HasStrictBackend,
		hasLineageBackend:   capabilities.HasLineageBackend,
	})
}
