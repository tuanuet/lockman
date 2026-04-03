package lockman

import (
	"context"
	"strings"
	"testing"
)

func TestDefineLockCreatesStableDefinitionID(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))

	if err := def.ForceRelease(context.Background(), nil, ""); err == nil {
		t.Fatal("expected ForceRelease to return error when not implemented")
	}

	id := def.stableID()
	if id == "" {
		t.Fatal("expected stable definition ID to be non-empty")
	}
	if !strings.HasPrefix(id, "contract") {
		t.Fatalf("expected stable ID to be based on name, got %q", id)
	}
}

func TestDefineLockRejectsEmptyName(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected DefineLock with empty name to panic")
		}
	}()

	DefineLock("", BindResourceID("order", func(v string) string { return v }))
}

func TestDefineLockRejectsMissingBinding(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected DefineLock with nil binding to panic")
		}
	}()

	DefineLock("order", Binding[string]{})
}

func TestDefineRunOnSharesDefinitionAcrossUseCases(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
	importUC := DefineRunOn("import", def)
	deleteUC := DefineRunOn("delete", def)

	if importUC.DefinitionID() != "import" {
		t.Fatalf("expected importUC.DefinitionID() to be 'import', got %q", importUC.DefinitionID())
	}
	if deleteUC.DefinitionID() != "delete" {
		t.Fatalf("expected deleteUC.DefinitionID() to be 'delete', got %q", deleteUC.DefinitionID())
	}
	if importUC.core.config.definitionRef != deleteUC.core.config.definitionRef {
		t.Fatal("expected use cases sharing a definition to share the same definitionRef pointer")
	}
}

func TestDefineClaimOnSharesDefinitionAcrossUseCases(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
	notifyUC := DefineClaimOn("notify", def)
	alertUC := DefineClaimOn("alert", def)

	if notifyUC.core.config.definitionRef != alertUC.core.config.definitionRef {
		t.Fatal("expected claim use cases sharing a definition to share the same definitionRef pointer")
	}
}

func TestDefineHoldOnRejectsStrictDefinition(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected DefineHoldOn with strict definition to panic")
		}
	}()

	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }), StrictDef())
	DefineHoldOn("hold", def)
}

func TestRunDefinitionIDRemainsPublicNameFacing(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
	uc := DefineRunOn("import", def)

	if uc.DefinitionID() != "import" {
		t.Fatalf("expected DefinitionID to return use-case name 'import', got %q", uc.DefinitionID())
	}
}

func TestDefinitionIDNeverExposesInternalHashedID(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
	uc := DefineRunOn("import", def)

	id := uc.DefinitionID()
	if strings.Contains(id, "#") || strings.Contains(id, "sha") || strings.Contains(id, "hash") {
		t.Fatalf("expected DefinitionID to never expose internal hashed ID, got %q", id)
	}
}
