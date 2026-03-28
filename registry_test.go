package lockman

import "testing"

func TestRegistryRejectsDuplicateUseCaseNames(t *testing.T) {
	reg := NewRegistry()

	a := DefineRun[string](
		"order.approve",
		BindResourceID("order", func(v string) string { return v }),
	)
	b := DefineRun[string](
		"order.approve",
		BindResourceID("order", func(v string) string { return v }),
	)

	if err := reg.Register(a, b); err == nil {
		t.Fatal("expected duplicate use case name rejection")
	}
}

func TestRegistryRejectsEmptyUseCaseName(t *testing.T) {
	reg := NewRegistry()
	uc := DefineRun[string](
		"   ",
		BindResourceID("order", func(v string) string { return v }),
	)

	if err := reg.Register(uc); err == nil {
		t.Fatal("expected empty use case name rejection")
	}
}

func TestRegistryRejectsUseCaseFromDifferentRegistry(t *testing.T) {
	regA := NewRegistry()
	regB := NewRegistry()
	uc := DefineRun[string](
		"order.approve",
		BindResourceID("order", func(v string) string { return v }),
	)

	if err := regA.Register(uc); err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	if err := regB.Register(uc); err == nil {
		t.Fatal("expected cross-registry registration rejection")
	}
}
