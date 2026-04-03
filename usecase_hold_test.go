package lockman

import (
	"errors"
	"testing"

	"github.com/tuanuet/lockman/internal/sdk"
)

func testHoldUseCase(name string) HoldUseCase[string] {
	def := DefineLock(name, BindResourceID("order", func(v string) string { return v }))
	return DefineHoldOn[string](name, def)
}

func TestHoldUseCaseWithBindsCanonicalResourceKey(t *testing.T) {
	uc := testHoldUseCase("order.hold")

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	if req.resourceKey != "order:123" {
		t.Fatalf("expected canonical resource key, got %q", req.resourceKey)
	}
}

func TestHoldUseCaseWithRejectsEmptyBoundResourceID(t *testing.T) {
	uc := testHoldUseCase("order.hold")

	_, err := uc.With("")
	if err == nil {
		t.Fatal("expected empty bound resource id to fail")
	}
	if !errors.Is(err, errEmptyBindingValue) {
		t.Fatalf("expected errEmptyBindingValue, got %v", err)
	}
}

func TestHoldUseCaseWithRejectsNilBinding(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected DefineLock with nil binding to panic")
		}
	}()

	def := DefineLock("order.hold", Binding[string]{})
	DefineHoldOn[string]("order.hold", def)
}

func TestHoldUseCaseWithRejectsEmptyOwnerOverride(t *testing.T) {
	uc := testHoldUseCase("order.hold")

	_, err := uc.With("123", OwnerID("   "))
	if err == nil {
		t.Fatal("expected empty owner override to fail")
	}
	if !errors.Is(err, ErrIdentityRequired) {
		t.Fatalf("expected ErrIdentityRequired, got %v", err)
	}
}

func TestHoldUseCaseWithSetsBoundToRegistryAfterRegistration(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCase("order.hold")
	if err := reg.Register(uc); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	if !req.boundToRegistry {
		t.Fatal("expected hold request to be bound to registry")
	}
	if sdk.RegistryLinkMismatch(reg.link, req.registryLink) {
		t.Fatal("expected hold request registry link to match registry")
	}
}

func TestHoldUseCaseWithNotBoundToRegistryBeforeRegistration(t *testing.T) {
	uc := testHoldUseCase("order.hold")

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	if req.boundToRegistry {
		t.Fatal("expected hold request to not be bound before registration")
	}
}

func TestHoldUseCaseForfeitWithPackagesToken(t *testing.T) {
	reg := NewRegistry()
	uc := testHoldUseCase("order.hold")
	if err := reg.Register(uc); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	req := uc.ForfeitWith("h1_abc123")
	if req.useCaseName != "order.hold" {
		t.Fatalf("expected use-case name order.hold, got %q", req.useCaseName)
	}
	if req.token != "h1_abc123" {
		t.Fatalf("expected raw token to be preserved, got %q", req.token)
	}
	if req.useCaseCore != uc.core {
		t.Fatal("expected use-case core to be carried")
	}
	if !req.boundToRegistry {
		t.Fatal("expected forfeit request to be bound to registry")
	}
	if sdk.RegistryLinkMismatch(reg.link, req.registryLink) {
		t.Fatal("expected forfeit request registry link to match registry")
	}
}
