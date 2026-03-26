package registry_test

import (
	"testing"

	"lockman/lockkit/definitions"
	"lockman/lockkit/registry"
)

func TestRegistryRejectsDuplicateDefinitionIDs(t *testing.T) {
	reg := registry.New()

	builder := definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"})
	def := definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		KeyBuilder:    builder,
	}

	if err := reg.Register(def); err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	if err := reg.Register(def); err == nil {
		t.Fatal("expected duplicate ID rejection")
	}
}

func TestRegistryValidateRejectsStrictWithoutFencing(t *testing.T) {
	reg := registry.New()

	builder := definitions.MustTemplateKeyBuilder("payment:{payment_id}", []string{"payment_id"})
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "PaymentLock",
		Kind:                 definitions.KindParent,
		Resource:             "payment",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionSync,
		KeyBuilder:           builder,
		FencingRequired:      false,
		BackendFailurePolicy: definitions.BackendFailClosed,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected strict validation failure")
	}
}

func TestRegistryRegisterRejectsEmptyID(t *testing.T) {
	reg := registry.New()

	builder := definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"})
	err := reg.Register(definitions.LockDefinition{
		ID:            "   ",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		KeyBuilder:    builder,
	})
	if err == nil {
		t.Fatal("expected empty ID rejection")
	}

	if err := reg.Validate(); err != nil {
		t.Fatalf("expected registry still valid after rejecting invalid definition: %v", err)
	}
}

func TestRegistryValidateRejectsDefinitionWithoutKeyBuilder(t *testing.T) {
	reg := registry.New()

	if err := reg.Register(definitions.LockDefinition{
		ID:            "BrokenLock",
		Kind:          definitions.KindParent,
		Resource:      "broken",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected missing key builder validation failure")
	}
}

func TestRegistryValidateRejectsInvalidFailOpenStrictDefinition(t *testing.T) {
	reg := registry.New()

	builder := definitions.MustTemplateKeyBuilder("payment:{payment_id}", []string{"payment_id"})
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "PaymentLock",
		Kind:                 definitions.KindParent,
		Resource:             "payment",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionSync,
		BackendFailurePolicy: definitions.BackendBestEffortOpen,
		FencingRequired:      true,
		KeyBuilder:           builder,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected strict fail-open validation failure")
	}
}

func TestRegistryValidateRejectsStrictDefinitionWithoutBackendFailurePolicy(t *testing.T) {
	reg := registry.New()

	builder := definitions.MustTemplateKeyBuilder("payment:{payment_id}", []string{"payment_id"})
	if err := reg.Register(definitions.LockDefinition{
		ID:              "StrictMissingPolicy",
		Kind:            definitions.KindParent,
		Resource:        "payment",
		Mode:            definitions.ModeStrict,
		ExecutionKind:   definitions.ExecutionSync,
		FencingRequired: true,
		KeyBuilder:      builder,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected strict missing backend policy validation failure")
	}
}

func TestRegistryValidateRejectsStrictDefinitionWithUnknownBackendFailurePolicy(t *testing.T) {
	reg := registry.New()

	builder := definitions.MustTemplateKeyBuilder("payment:{payment_id}", []string{"payment_id"})
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "StrictUnknownPolicy",
		Kind:                 definitions.KindParent,
		Resource:             "payment",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionSync,
		FencingRequired:      true,
		KeyBuilder:           builder,
		BackendFailurePolicy: definitions.BackendFailurePolicy("unknown_policy"),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected strict unknown backend policy validation failure")
	}
}

func TestRegistryValidateAcceptsValidDefinition(t *testing.T) {
	reg := registry.New()

	builder := definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"})
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		KeyBuilder:    builder,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := reg.Validate(); err != nil {
		t.Fatalf("expected valid registry: %v", err)
	}
}

func TestRegistryClonesTagsBeforeStorage(t *testing.T) {
	reg := registry.New()

	builder := definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"})
	tags := map[string]string{}
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		KeyBuilder:    builder,
		Tags:          tags,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	tags["mutate"] = "value"

	stored := reg.MustGet("OrderLock")
	if len(stored.Tags) != 0 {
		t.Fatalf("expected stored tags to remain empty, got %v", stored.Tags)
	}

	stored.Tags["new"] = "value"
	if _, ok := tags["new"]; ok {
		t.Fatalf("expected original tags map to be isolated")
	}
}

func TestRegistryRegisterCompositeStoresDefinition(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(validParentDefinition()); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "transfer",
		Members:          []string{"account.parent"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   1,
		ExecutionKind:    definitions.ExecutionBoth,
	})
	if err != nil {
		t.Fatalf("RegisterComposite returned error: %v", err)
	}

	got := reg.MustGetComposite("transfer")
	if got.ID != "transfer" {
		t.Fatalf("expected composite definition, got %#v", got)
	}
}

func TestRegistryValidateRejectsUnknownParentRef(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "account.child",
		Kind:                 definitions.KindChild,
		Resource:             "account",
		Mode:                 definitions.ModeStandard,
		ExecutionKind:        definitions.ExecutionSync,
		ParentRef:            "account.missing",
		OverlapPolicy:        definitions.OverlapReject,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("account:{account_id}:child", []string{"account_id"}),
		BackendFailurePolicy: definitions.BackendBestEffortOpen,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected unknown ParentRef validation failure")
	}
}

func TestRegistryValidateRejectsUnsupportedOverlapPolicy(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(validParentDefinition()); err != nil {
		t.Fatalf("register parent failed: %v", err)
	}

	if err := reg.Register(definitions.LockDefinition{
		ID:                   "account.child",
		Kind:                 definitions.KindChild,
		Resource:             "account",
		Mode:                 definitions.ModeStandard,
		ExecutionKind:        definitions.ExecutionSync,
		ParentRef:            "account.parent",
		OverlapPolicy:        definitions.OverlapEscalate,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("account:{account_id}:child", []string{"account_id"}),
		BackendFailurePolicy: definitions.BackendBestEffortOpen,
	}); err != nil {
		t.Fatalf("register child failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected unsupported overlap policy validation failure")
	}
}

func TestRegistryValidateRejectsCompositeUnknownMember(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(validParentDefinition()); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := reg.RegisterComposite(validCompositeDefinition("account.parent", "account.unknown")); err != nil {
		t.Fatalf("register composite failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected unknown composite member validation failure")
	}
}

func TestRegistryValidateRejectsCompositeWithStrictMember(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "account.strict",
		Kind:                 definitions.KindParent,
		Resource:             "account",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionSync,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("account:{account_id}:strict", []string{"account_id"}),
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  true,
	}); err != nil {
		t.Fatalf("register strict member failed: %v", err)
	}

	if err := reg.RegisterComposite(validCompositeDefinition("account.strict")); err != nil {
		t.Fatalf("register composite failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected strict composite member validation failure")
	}
}

func TestRegistryValidateRejectsCompositeMixedModes(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(validParentDefinition()); err != nil {
		t.Fatalf("register standard member failed: %v", err)
	}
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "account.strict",
		Kind:                 definitions.KindParent,
		Resource:             "account",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionSync,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("account:{account_id}:strict", []string{"account_id"}),
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  true,
	}); err != nil {
		t.Fatalf("register strict member failed: %v", err)
	}

	if err := reg.RegisterComposite(validCompositeDefinition("account.parent", "account.strict")); err != nil {
		t.Fatalf("register composite failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected mixed-mode composite validation failure")
	}
}

func TestRegistryValidateRejectsCompositeUnsupportedPolicies(t *testing.T) {
	tests := []struct {
		name      string
		composite definitions.CompositeDefinition
	}{
		{
			name: "ordering policy",
			composite: definitions.CompositeDefinition{
				ID:               "transfer-ordering",
				Members:          []string{"account.parent"},
				OrderingPolicy:   definitions.OrderingPolicy("random"),
				AcquirePolicy:    definitions.AcquireAllOrNothing,
				EscalationPolicy: definitions.EscalationReject,
				ModeResolution:   definitions.ModeResolutionHomogeneous,
				MaxMemberCount:   1,
				ExecutionKind:    definitions.ExecutionBoth,
			},
		},
		{
			name: "acquire policy",
			composite: definitions.CompositeDefinition{
				ID:               "transfer-acquire",
				Members:          []string{"account.parent"},
				OrderingPolicy:   definitions.OrderingCanonical,
				AcquirePolicy:    definitions.AcquirePolicy("partial"),
				EscalationPolicy: definitions.EscalationReject,
				ModeResolution:   definitions.ModeResolutionHomogeneous,
				MaxMemberCount:   1,
				ExecutionKind:    definitions.ExecutionBoth,
			},
		},
		{
			name: "escalation policy",
			composite: definitions.CompositeDefinition{
				ID:               "transfer-escalation",
				Members:          []string{"account.parent"},
				OrderingPolicy:   definitions.OrderingCanonical,
				AcquirePolicy:    definitions.AcquireAllOrNothing,
				EscalationPolicy: definitions.EscalationPolicy("allow"),
				ModeResolution:   definitions.ModeResolutionHomogeneous,
				MaxMemberCount:   1,
				ExecutionKind:    definitions.ExecutionBoth,
			},
		},
		{
			name: "mode resolution",
			composite: definitions.CompositeDefinition{
				ID:               "transfer-resolution",
				Members:          []string{"account.parent"},
				OrderingPolicy:   definitions.OrderingCanonical,
				AcquirePolicy:    definitions.AcquireAllOrNothing,
				EscalationPolicy: definitions.EscalationReject,
				ModeResolution:   definitions.ModeResolution("heterogeneous"),
				MaxMemberCount:   1,
				ExecutionKind:    definitions.ExecutionBoth,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := registry.New()
			if err := reg.Register(validParentDefinition()); err != nil {
				t.Fatalf("register failed: %v", err)
			}
			if err := reg.RegisterComposite(tt.composite); err != nil {
				t.Fatalf("register composite failed: %v", err)
			}

			if err := reg.Validate(); err == nil {
				t.Fatalf("expected composite %s validation failure", tt.name)
			}
		})
	}
}

func TestRegistryValidateRejectsCompositeExceedingMaxMemberCount(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(validParentDefinition()); err != nil {
		t.Fatalf("register first member failed: %v", err)
	}
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "account.parent.secondary",
		Kind:                 definitions.KindParent,
		Resource:             "account",
		Mode:                 definitions.ModeStandard,
		ExecutionKind:        definitions.ExecutionSync,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("account:{account_id}:parent-secondary", []string{"account_id"}),
		BackendFailurePolicy: definitions.BackendBestEffortOpen,
	}); err != nil {
		t.Fatalf("register second member failed: %v", err)
	}

	if err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "transfer",
		Members:          []string{"account.parent", "account.parent.secondary"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   1,
		ExecutionKind:    definitions.ExecutionBoth,
	}); err != nil {
		t.Fatalf("register composite failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected composite max member count validation failure")
	}
}

func TestRegistryRegisterCompositeRejectsIDCollisionWithDefinition(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(validParentDefinition()); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "account.parent",
		Members:          []string{"account.parent"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   1,
		ExecutionKind:    definitions.ExecutionBoth,
	})
	if err == nil {
		t.Fatal("expected composite/definition ID collision rejection")
	}
}

func TestRegistryValidateRejectsStrictAsyncWithoutIdempotency(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "account.strict-async",
		Kind:                 definitions.KindParent,
		Resource:             "account",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionAsync,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("account:{account_id}:strict-async", []string{"account_id"}),
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  false,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected strict async idempotency validation failure")
	}
}

func TestRegistryValidateRejectsStrictBothWithoutIdempotency(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "account.strict-both",
		Kind:                 definitions.KindParent,
		Resource:             "account",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionBoth,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("account:{account_id}:strict-both", []string{"account_id"}),
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  false,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected strict both idempotency validation failure")
	}
}

func validParentDefinition() definitions.LockDefinition {
	return definitions.LockDefinition{
		ID:                   "account.parent",
		Kind:                 definitions.KindParent,
		Resource:             "account",
		Mode:                 definitions.ModeStandard,
		ExecutionKind:        definitions.ExecutionSync,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("account:{account_id}:parent", []string{"account_id"}),
		BackendFailurePolicy: definitions.BackendBestEffortOpen,
	}
}

func validCompositeDefinition(members ...string) definitions.CompositeDefinition {
	return definitions.CompositeDefinition{
		ID:               "transfer",
		Members:          members,
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   len(members),
		ExecutionKind:    definitions.ExecutionBoth,
	}
}
