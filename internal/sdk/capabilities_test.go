package sdk

import (
	"errors"
	"testing"
)

func TestValidateCapabilitiesRejectsClaimUseCaseWithoutIdempotencyStore(t *testing.T) {
	link := newRegistryLink()
	useCases := []useCase{
		newUseCase(
			"order.process",
			useCaseKindClaim,
			capabilityRequirements{requiresIdempotency: true},
			link,
		),
	}

	err := validateCapabilities(useCases, backendCapabilities{})
	if !errors.Is(err, errIdempotencyCapabilityRequired) {
		t.Fatalf("expected idempotency capability error, got %v", err)
	}
}

func TestValidateCapabilitiesRejectsMissingStrictAndLineageSupport(t *testing.T) {
	link := newRegistryLink()
	useCases := []useCase{
		newUseCase(
			"order.strict",
			useCaseKindRun,
			capabilityRequirements{requiresStrict: true},
			link,
		),
		newUseCase(
			"order.child",
			useCaseKindClaim,
			capabilityRequirements{requiresLineage: true},
			link,
		),
	}

	err := validateCapabilities(useCases, backendCapabilities{})
	if !errors.Is(err, errStrictCapabilityRequired) {
		t.Fatalf("expected strict capability error first, got %v", err)
	}
}

func TestValidateCapabilitiesPassesWhenRequirementsMet(t *testing.T) {
	link := newRegistryLink()
	useCases := []useCase{
		newUseCase(
			"order.process",
			useCaseKindClaim,
			capabilityRequirements{
				requiresIdempotency: true,
				requiresStrict:      true,
				requiresLineage:     true,
			},
			link,
		),
	}

	err := validateCapabilities(useCases, backendCapabilities{
		hasIdempotencyStore: true,
		hasStrictBackend:    true,
		hasLineageBackend:   true,
	})
	if err != nil {
		t.Fatalf("expected capabilities validation success, got %v", err)
	}
}

func TestExportedValidateCapabilitiesIsAvailable(t *testing.T) {
	link := NewRegistryLink()
	useCases := []UseCase{
		NewUseCase(
			"order.process",
			UseCaseKindClaim,
			CapabilityRequirements{RequiresIdempotency: true},
			link,
		),
	}

	err := ValidateCapabilities(useCases, BackendCapabilities{
		HasIdempotencyStore: true,
	})
	if err != nil {
		t.Fatalf("expected exported validation to pass, got %v", err)
	}
}
