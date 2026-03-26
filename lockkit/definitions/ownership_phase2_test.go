package definitions

import "testing"

func TestMessageClaimRequestCarriesIdempotencyKey(t *testing.T) {
	req := MessageClaimRequest{IdempotencyKey: "msg:123"}
	if req.IdempotencyKey == "" {
		t.Fatal("expected idempotency key to be preserved")
	}
}

func TestClaimContextSupportsCompositeResources(t *testing.T) {
	claim := ClaimContext{
		ResourceKeys:   []string{"account:a", "account:b"},
		IdempotencyKey: "msg:123",
	}

	if len(claim.ResourceKeys) != 2 {
		t.Fatalf("expected composite resource keys, got %d", len(claim.ResourceKeys))
	}
}
