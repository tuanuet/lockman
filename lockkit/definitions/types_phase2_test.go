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

	if string(def.OrderingPolicy) != "canonical" {
		t.Fatalf("expected ordering policy %q, got %q", "canonical", def.OrderingPolicy)
	}
	if string(def.AcquirePolicy) != "all_or_nothing" {
		t.Fatalf("expected acquire policy %q, got %q", "all_or_nothing", def.AcquirePolicy)
	}
	if string(def.EscalationPolicy) != "reject" {
		t.Fatalf("expected escalation policy %q, got %q", "reject", def.EscalationPolicy)
	}
	if string(def.ModeResolution) != "homogeneous" {
		t.Fatalf("expected mode resolution %q, got %q", "homogeneous", def.ModeResolution)
	}
}

func TestPhase2LockDefinitionCarriesOverlapPolicy(t *testing.T) {
	def := LockDefinition{
		ID:            "order.item",
		Kind:          KindChild,
		OverlapPolicy: OverlapReject,
	}

	if def.OverlapPolicy != OverlapReject {
		t.Fatalf("expected overlap policy to be preserved, got %q", def.OverlapPolicy)
	}

	if string(OverlapReject) != "reject" {
		t.Fatalf("expected overlap reject value %q, got %q", "reject", OverlapReject)
	}
	if string(OverlapEscalate) != "escalate" {
		t.Fatalf("expected overlap escalate value %q, got %q", "escalate", OverlapEscalate)
	}
}
