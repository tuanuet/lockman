package definitions

import "testing"

func TestCompositeDefinitionCarriesPhase2Shape(t *testing.T) {
	def := CompositeDefinition{
		ID:               "transfer",
		Members:          []string{"account.debit", "account.credit"},
		OrderingPolicy:   OrderingCanonical,
		AcquirePolicy:    AcquireAllOrNothing,
		EscalationPolicy: EscalationReject,
		ModeResolution:   ModeResolutionHomogeneous,
		MaxMemberCount:   2,
		ExecutionKind:    ExecutionBoth,
	}

	if len(def.Members) != 2 {
		t.Fatalf("expected composite members, got %d", len(def.Members))
	}
}

func TestLockDefinitionCarriesOverlapPolicy(t *testing.T) {
	def := LockDefinition{
		ID:            "order.item",
		Kind:          KindChild,
		OverlapPolicy: OverlapReject,
	}

	if def.OverlapPolicy != OverlapReject {
		t.Fatalf("expected overlap policy to be preserved, got %q", def.OverlapPolicy)
	}
}
