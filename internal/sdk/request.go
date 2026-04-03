package sdk

import "strings"

type claimDelivery struct {
	messageID     string
	consumerGroup string
	attempt       int
}

type runRequest struct {
	useCaseID    useCaseID
	publicName   string
	resourceKey  string
	ownerID      string
	registryLink registryLink
}

type claimRequest struct {
	useCaseID    useCaseID
	publicName   string
	resourceKey  string
	ownerID      string
	delivery     claimDelivery
	idempotent   bool
	registryLink registryLink
}

// ResourceKeyInputKey is the canonical lock key input map key for normalized requests.
const ResourceKeyInputKey = "resource_key"

// DefinitionIDInputKey is the input map key carrying a shared definition ID for composite members.
const DefinitionIDInputKey = "definition_id"

const resourceKeyInputKey = ResourceKeyInputKey

// ClaimDelivery carries normalized claim-delivery metadata.
type ClaimDelivery struct {
	MessageID     string
	ConsumerGroup string
	Attempt       int
}

// RunRequest is a normalized run request used by package lockman internals.
type RunRequest struct {
	internal runRequest
}

// ClaimRequest is a normalized claim request used by package lockman internals.
type ClaimRequest struct {
	internal claimRequest
}

func bindRunRequest(uc useCase, resourceKey string, ownerID string) runRequest {
	return runRequest{
		useCaseID:    uc.id,
		publicName:   uc.publicName,
		resourceKey:  strings.TrimSpace(resourceKey),
		ownerID:      strings.TrimSpace(ownerID),
		registryLink: uc.registryLink,
	}
}

func bindClaimRequest(uc useCase, resourceKey string, ownerID string, delivery claimDelivery) claimRequest {
	return claimRequest{
		useCaseID:    uc.id,
		publicName:   uc.publicName,
		resourceKey:  strings.TrimSpace(resourceKey),
		ownerID:      strings.TrimSpace(ownerID),
		delivery:     delivery,
		idempotent:   uc.requirements.requiresIdempotency,
		registryLink: uc.registryLink,
	}
}

// BindRunRequest binds a normalized use case and key into an internal SDK run request.
func BindRunRequest(uc UseCase, resourceKey string, ownerID string) RunRequest {
	return RunRequest{
		internal: bindRunRequest(uc.internal, resourceKey, ownerID),
	}
}

// BindClaimRequest binds a normalized use case and delivery metadata into an internal SDK claim request.
func BindClaimRequest(uc UseCase, resourceKey string, ownerID string, delivery ClaimDelivery) ClaimRequest {
	return ClaimRequest{
		internal: bindClaimRequest(uc.internal, resourceKey, ownerID, claimDelivery{
			messageID:     strings.TrimSpace(delivery.MessageID),
			consumerGroup: strings.TrimSpace(delivery.ConsumerGroup),
			attempt:       delivery.Attempt,
		}),
	}
}
