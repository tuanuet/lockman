package lockman

import "testing"

func TestRegistryRejectsDuplicateUseCaseNames(t *testing.T) {
	reg := NewRegistry()

	def := DefineLock("order.approve", BindResourceID("order", func(v string) string { return v }))
	a := DefineRunOn[string]("order.approve", def)
	b := DefineRunOn[string]("order.approve", def)

	if err := reg.Register(a, b); err == nil {
		t.Fatal("expected duplicate use case name rejection")
	}
}

func TestRegistryRejectsEmptyUseCaseName(t *testing.T) {
	reg := NewRegistry()
	def := DefineLock("order", BindResourceID("order", func(v string) string { return v }))
	uc := DefineRunOn[string]("   ", def)

	if err := reg.Register(uc); err == nil {
		t.Fatal("expected empty use case name rejection")
	}
}

func TestRegistryRejectsUseCaseFromDifferentRegistry(t *testing.T) {
	regA := NewRegistry()
	regB := NewRegistry()
	def := DefineLock("order.approve", BindResourceID("order", func(v string) string { return v }))
	uc := DefineRunOn[string]("order.approve", def)

	if err := regA.Register(uc); err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	if err := regB.Register(uc); err == nil {
		t.Fatal("expected cross-registry registration rejection")
	}
}

func TestRegistryRegisterIsAtomicOnFailure(t *testing.T) {
	reg := NewRegistry()
	def := DefineLock("order.approve", BindResourceID("order", func(v string) string { return v }))
	valid := DefineRunOn[string]("order.approve", def)
	invalid := DefineRunOn[string]("   ", def)

	if err := reg.Register(valid, invalid); err == nil {
		t.Fatal("expected batch registration failure")
	}
	if len(reg.byName) != 0 {
		t.Fatalf("expected registry to remain empty, got %d entries", len(reg.byName))
	}
	if valid.core.registry != nil {
		t.Fatal("expected valid use case to remain unbound after batch failure")
	}
}
