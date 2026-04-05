package registry_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/lockkit/definitions"
	"github.com/tuanuet/lockman/lockkit/registry"
)

func TestRegistryRejectsDuplicateDefinitionIDs(t *testing.T) {
	reg := registry.New()

	builder := definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"})
	def := definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
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
		Kind:                 backend.KindParent,
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
		Kind:          backend.KindParent,
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
		Kind:          backend.KindParent,
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
		Kind:                 backend.KindParent,
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
		Kind:            backend.KindParent,
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
		Kind:                 backend.KindParent,
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
		Kind:          backend.KindParent,
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
		Kind:          backend.KindParent,
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
		Kind:                 backend.KindParent,
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
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "account.strict.secondary",
		Kind:                 backend.KindParent,
		Resource:             "account",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionSync,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("account:{account_id}:strict-secondary", []string{"account_id"}),
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  true,
	}); err != nil {
		t.Fatalf("register second strict member failed: %v", err)
	}

	if err := reg.RegisterComposite(validCompositeDefinition("account.strict", "account.strict.secondary")); err != nil {
		t.Fatalf("register composite failed: %v", err)
	}

	err := reg.Validate()
	if err == nil {
		t.Fatal("expected strict composite member validation failure")
	}
	if !strings.Contains(err.Error(), "cannot include strict members") {
		t.Fatalf("expected strict member validation error, got: %v", err)
	}
}

func TestRegistryValidateRejectsCompositeMixedModes(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(validParentDefinition()); err != nil {
		t.Fatalf("register standard member failed: %v", err)
	}
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "account.strict",
		Kind:                 backend.KindParent,
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

	err := reg.Validate()
	if err == nil {
		t.Fatal("expected mixed-mode composite validation failure")
	}
	if !strings.Contains(err.Error(), "homogeneous mode") {
		t.Fatalf("expected mixed-mode validation error, got: %v", err)
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
		Kind:                 backend.KindParent,
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
		Kind:                 backend.KindParent,
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
		Kind:                 backend.KindParent,
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

func TestRegistryValidateRejectsBrokenLineageChain(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          backend.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "order",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("item:{item_id}", []string{"item_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "preserve parent template prefix") {
		t.Fatalf("expected lineage prefix validation error, got %v", err)
	}
}

func TestRegistryValidateRejectsNonRejectOverlapForChildLineage(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          backend.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "order",
		OverlapPolicy: definitions.OverlapEscalate,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "only support overlap policy reject") {
		t.Fatalf("expected reject-only overlap validation error, got %v", err)
	}
}

func TestRegistryValidateRejectsUnknownParentRef(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          backend.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "missing-parent",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "unknown parent") {
		t.Fatalf("expected unknown parent validation error, got %v", err)
	}
}

func TestRegistryValidateRejectsParentRefCycle(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "item",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          backend.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "order",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle validation error, got %v", err)
	}
}

func TestRegistryValidateAcceptsGrandchildLineageChain(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          backend.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "order",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "allocation",
		Kind:          backend.KindChild,
		Resource:      "allocation",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "item",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}:allocation:{allocation_id}", []string{"order_id", "item_id", "allocation_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})

	if err := reg.Validate(); err != nil {
		t.Fatalf("expected recursive grandchild lineage to validate, got %v", err)
	}
}

func TestRegistryValidateRejectsNonTemplateChildLineageBuilder(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          backend.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "order",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder: stubKeyBuilder{
			fields: []string{"order_id", "item_id"},
		},
		ExecutionKind: definitions.ExecutionSync,
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "template-backed key builders") {
		t.Fatalf("expected template-backed lineage validation error, got %v", err)
	}
}

func TestRegistryValidateRejectsNonTemplateParentLineageBuilder(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:       "order",
		Kind:     backend.KindParent,
		Resource: "order",
		Mode:     definitions.ModeStandard,
		LeaseTTL: 30 * time.Second,
		KeyBuilder: stubKeyBuilder{
			fields: []string{"order_id"},
		},
		ExecutionKind: definitions.ExecutionSync,
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          backend.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "order",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "template-backed key builders") {
		t.Fatalf("expected template-backed lineage validation error, got %v", err)
	}
}

func TestRegistryValidateRejectsChildMissingParentFields(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})
	childBuilder, err := definitions.NewTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"item_id"})
	if err != nil {
		t.Fatalf("failed to create child builder: %v", err)
	}
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          backend.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "order",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    childBuilder,
		ExecutionKind: definitions.ExecutionSync,
	})

	err = reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "include parent template fields") {
		t.Fatalf("expected lineage parent field validation error, got %v", err)
	}
}

func TestRegistryValidateRejectsLineageWithNonStandardMode(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:                   "item",
		Kind:                 backend.KindChild,
		Resource:             "item",
		Mode:                 definitions.ModeStrict,
		LeaseTTL:             30 * time.Second,
		ParentRef:            "order",
		OverlapPolicy:        definitions.OverlapReject,
		ExecutionKind:        definitions.ExecutionSync,
		FencingRequired:      true,
		BackendFailurePolicy: definitions.BackendFailClosed,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "only support standard mode") {
		t.Fatalf("expected lineage mode validation error, got %v", err)
	}
}

func TestRegistryValidateRejectsStrictChildWithParentRefRegression(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:                   "item",
		Kind:                 backend.KindChild,
		Resource:             "item",
		Mode:                 definitions.ModeStrict,
		LeaseTTL:             30 * time.Second,
		ParentRef:            "order",
		OverlapPolicy:        definitions.OverlapReject,
		ExecutionKind:        definitions.ExecutionSync,
		FencingRequired:      true,
		BackendFailurePolicy: definitions.BackendFailClosed,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "only support standard mode") {
		t.Fatalf("expected strict child+parent regression rejection, got %v", err)
	}
}

func TestRegistryValidateRejectsStandardChildWithStrictParentRegression(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:                   "order",
		Kind:                 backend.KindParent,
		Resource:             "order",
		Mode:                 definitions.ModeStrict,
		LeaseTTL:             30 * time.Second,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		ExecutionKind:        definitions.ExecutionSync,
		FencingRequired:      true,
		BackendFailurePolicy: definitions.BackendFailClosed,
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          backend.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "order",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "only support standard mode") {
		t.Fatalf("expected standard child+strict parent regression rejection, got %v", err)
	}
}

func TestGetReturnsDefinitionWithoutClone(t *testing.T) {
	reg := registry.New()
	tags := map[string]string{"env": "prod"}
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		Tags:          tags,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := reg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	def, ok := reg.Get("OrderLock")
	if !ok {
		t.Fatal("expected Get to return true")
	}
	if def.ID != "OrderLock" {
		t.Fatalf("expected ID OrderLock, got %s", def.ID)
	}

	_, ok = reg.Get("nonexistent")
	if ok {
		t.Fatal("expected Get to return false for unknown ID")
	}
}

func TestGetCompositeReturnsDefinitionWithoutClone(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "alpha",
		Kind:          backend.KindParent,
		Resource:      "alpha",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("alpha:{id}", []string{"id"}),
	}); err != nil {
		t.Fatalf("register member: %v", err)
	}
	if err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:      "transfer",
		Members: []string{"alpha"},
	}); err != nil {
		t.Fatalf("register composite: %v", err)
	}

	def, ok := reg.GetComposite("transfer")
	if !ok {
		t.Fatal("expected GetComposite to return true")
	}
	if def.ID != "transfer" {
		t.Fatalf("expected ID transfer, got %s", def.ID)
	}

	_, ok = reg.GetComposite("nonexistent")
	if ok {
		t.Fatal("expected GetComposite to return false for unknown ID")
	}
}

func TestRequiresStrictRuntimeDriverIgnoresAsyncOnlyStrictDefinitions(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:                   "StrictAsyncOnly",
		Kind:                 backend.KindParent,
		Resource:             "order",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionAsync,
		LeaseTTL:             time.Second,
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  true,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	})

	if registry.RequiresStrictRuntimeDriver(reg) {
		t.Fatal("runtime strict gate should ignore async-only strict definitions")
	}
	if !registry.RequiresStrictWorkerDriver(reg) {
		t.Fatal("worker strict gate should include async-only strict definitions")
	}
}

func TestRequiresStrictRuntimeDriverIncludesSyncAndBothStrictDefinitions(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:                   "StrictBoth",
		Kind:                 backend.KindParent,
		Resource:             "order",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionBoth,
		LeaseTTL:             time.Second,
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  true,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	})

	if !registry.RequiresStrictRuntimeDriver(reg) {
		t.Fatal("runtime strict gate should include strict both definitions")
	}
}

func TestRequiresStrictWorkerDriverIgnoresSyncOnlyStrictDefinitions(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:                   "StrictSyncOnly",
		Kind:                 backend.KindParent,
		Resource:             "order",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionSync,
		LeaseTTL:             time.Second,
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	})

	if registry.RequiresStrictWorkerDriver(reg) {
		t.Fatal("worker strict gate should ignore sync-only strict definitions")
	}
}

func TestRegistryValidateRejectsLineageWithNonStandardParentMode(t *testing.T) {
	reg := registry.New()
	mustRegister(t, reg, definitions.LockDefinition{
		ID:                   "order",
		Kind:                 backend.KindParent,
		Resource:             "order",
		Mode:                 definitions.ModeStrict,
		LeaseTTL:             30 * time.Second,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		ExecutionKind:        definitions.ExecutionSync,
		FencingRequired:      true,
		BackendFailurePolicy: definitions.BackendFailClosed,
	})
	mustRegister(t, reg, definitions.LockDefinition{
		ID:            "item",
		Kind:          backend.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "order",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
		ExecutionKind: definitions.ExecutionSync,
	})

	err := reg.Validate()
	if err == nil || !strings.Contains(err.Error(), "only support standard mode") {
		t.Fatalf("expected lineage mode validation error, got %v", err)
	}
}

func validParentDefinition() definitions.LockDefinition {
	return definitions.LockDefinition{
		ID:                   "account.parent",
		Kind:                 backend.KindParent,
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

func mustRegister(t *testing.T, reg *registry.Registry, def definitions.LockDefinition) {
	t.Helper()
	if err := reg.Register(def); err != nil {
		t.Fatalf("failed to register %q: %v", def.ID, err)
	}
}

type stubKeyBuilder struct {
	fields []string
}

func (s stubKeyBuilder) RequiredFields() []string {
	fields := make([]string, len(s.fields))
	copy(fields, s.fields)
	return fields
}

func (s stubKeyBuilder) Build(input map[string]string) (string, error) {
	return "stub", nil
}
