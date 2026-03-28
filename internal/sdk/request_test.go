package sdk

import (
	"testing"
)

func TestRunRequestCarriesUseCaseIdentityAndRegistryLink(t *testing.T) {
	link := newRegistryLink()
	uc := newUseCase("order.approve", useCaseKindRun, capabilityRequirements{}, link)

	req := bindRunRequest(uc, "order:123", "owner-override")
	if req.useCaseID != uc.id {
		t.Fatalf("expected use case id %q, got %q", uc.id, req.useCaseID)
	}
	if req.publicName != "order.approve" {
		t.Fatalf("expected public name %q, got %q", "order.approve", req.publicName)
	}
	if req.registryLink != link {
		t.Fatal("expected request to carry originating registry link")
	}
	if req.ownerID != "owner-override" {
		t.Fatalf("expected owner override %q, got %q", "owner-override", req.ownerID)
	}

	translated := translateRun(req)
	if translated.DefinitionID != string(uc.id) {
		t.Fatalf("expected translated definition id %q, got %q", uc.id, translated.DefinitionID)
	}
	if translated.KeyInput[resourceKeyInputKey] != "order:123" {
		t.Fatalf("expected canonical resource key, got %+v", translated.KeyInput)
	}
	if translated.Ownership.OwnerID != "owner-override" {
		t.Fatalf("expected translated owner %q, got %q", "owner-override", translated.Ownership.OwnerID)
	}
}

func TestClaimRequestCarriesDeliveryMetadataAndTranslatesWhenIdempotent(t *testing.T) {
	link := newRegistryLink()
	uc := newUseCase("order.process", useCaseKindClaim, capabilityRequirements{requiresIdempotency: true}, link)

	req := bindClaimRequest(
		uc,
		"order:123",
		"worker-1",
		claimDelivery{
			messageID:     "msg-1",
			consumerGroup: "orders",
			attempt:       2,
		},
	)

	translated := translateClaim(req)
	if translated.DefinitionID != string(uc.id) {
		t.Fatalf("expected translated definition id %q, got %q", uc.id, translated.DefinitionID)
	}
	if translated.Ownership.MessageID != "msg-1" {
		t.Fatalf("expected message id, got %q", translated.Ownership.MessageID)
	}
	if translated.Ownership.ConsumerGroup != "orders" {
		t.Fatalf("expected consumer group, got %q", translated.Ownership.ConsumerGroup)
	}
	if translated.Ownership.Attempt != 2 {
		t.Fatalf("expected attempt 2, got %d", translated.Ownership.Attempt)
	}
	if translated.IdempotencyKey != "msg-1" {
		t.Fatalf("expected idempotency key from message id, got %q", translated.IdempotencyKey)
	}
}

func TestClaimRequestLeavesIdempotencyKeyEmptyWhenUseCaseIsNotIdempotent(t *testing.T) {
	link := newRegistryLink()
	uc := newUseCase("order.process", useCaseKindClaim, capabilityRequirements{}, link)

	req := bindClaimRequest(
		uc,
		"order:123",
		"worker-1",
		claimDelivery{
			messageID:     "msg-1",
			consumerGroup: "orders",
			attempt:       2,
		},
	)

	translated := translateClaim(req)
	if translated.IdempotencyKey != "" {
		t.Fatalf("expected empty idempotency key, got %q", translated.IdempotencyKey)
	}
}

func TestRegistryLinkMismatchIsDetected(t *testing.T) {
	left := newRegistryLink()
	right := newRegistryLink()
	if !registryLinkMismatch(left, right) {
		t.Fatal("expected registry mismatch to be detected")
	}
	if registryLinkMismatch(left, left) {
		t.Fatal("expected matching registry links to not mismatch")
	}
}

func TestInternalAPIMinimumIsAvailableForLockmanPackage(t *testing.T) {
	link := NewRegistryLink()
	uc := NewUseCase(
		"order.approve",
		UseCaseKindRun,
		CapabilityRequirements{},
		link,
	)
	req := BindRunRequest(uc, "order:123", "owner-a")
	translated := TranslateRun(req)
	if translated.KeyInput[ResourceKeyInputKey] != "order:123" {
		t.Fatalf("expected translated key input via exported API, got %+v", translated.KeyInput)
	}
}
