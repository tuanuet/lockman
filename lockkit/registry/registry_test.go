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
