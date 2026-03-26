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
