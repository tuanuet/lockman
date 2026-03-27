package lineage

import (
	"errors"
	"testing"
	"time"

	"lockman/lockkit/definitions"
)

func TestResolveAcquirePlanReturnsAncestorKeys(t *testing.T) {
	defs := map[string]definitions.LockDefinition{
		"order": {
			ID:         "order",
			Kind:       definitions.KindParent,
			Resource:   "order",
			Mode:       definitions.ModeStandard,
			LeaseTTL:   30 * time.Second,
			KeyBuilder: definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		},
		"item": {
			ID:            "item",
			Kind:          definitions.KindChild,
			Resource:      "item",
			Mode:          definitions.ModeStandard,
			LeaseTTL:      30 * time.Second,
			ParentRef:     "order",
			OverlapPolicy: definitions.OverlapReject,
			KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
		},
	}

	plan, err := ResolveAcquirePlan(defs["item"], defs, map[string]string{
		"order_id": "123",
		"item_id":  "line-1",
	})
	if err != nil {
		t.Fatalf("ResolveAcquirePlan returned error: %v", err)
	}
	if plan.ResourceKey != "order:123:item:line-1" {
		t.Fatalf("unexpected resource key: %q", plan.ResourceKey)
	}
	if len(plan.AncestorKeys) != 1 || plan.AncestorKeys[0].ResourceKey != "order:123" {
		t.Fatalf("unexpected ancestors: %#v", plan.AncestorKeys)
	}
}

func TestResolveAcquirePlanReturnsRootFirstAncestorsForGrandchild(t *testing.T) {
	defs := lineageDefinitions()
	defs["allocation"] = definitions.LockDefinition{
		ID:            "allocation",
		Kind:          definitions.KindChild,
		Resource:      "allocation",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "item",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}:allocation:{allocation_id}", []string{"order_id", "item_id", "allocation_id"}),
	}

	plan, err := ResolveAcquirePlan(defs["allocation"], defs, map[string]string{
		"order_id":      "123",
		"item_id":       "line-1",
		"allocation_id": "alloc-9",
	})
	if err != nil {
		t.Fatalf("ResolveAcquirePlan returned error: %v", err)
	}
	if got, want := len(plan.AncestorKeys), 2; got != want {
		t.Fatalf("expected %d ancestors, got %d", want, got)
	}
	if plan.AncestorKeys[0].ResourceKey != "order:123" || plan.AncestorKeys[1].ResourceKey != "order:123:item:line-1" {
		t.Fatalf("expected root-first ordering, got %#v", plan.AncestorKeys)
	}
}

func TestResolveAcquirePlanRejectsMissingParentDefinition(t *testing.T) {
	defs := lineageDefinitions()
	defs["item"] = definitions.LockDefinition{
		ID:            "item",
		Kind:          definitions.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "missing",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
	}

	_, err := ResolveAcquirePlan(defs["item"], defs, map[string]string{
		"order_id": "123",
		"item_id":  "line-1",
	})
	if err == nil || err.Error() == "" {
		t.Fatal("expected missing parent error")
	}
}

func TestResolveAcquirePlanRejectsCyclicParentRefs(t *testing.T) {
	defs := lineageDefinitions()
	defs["order"] = definitions.LockDefinition{
		ID:         "order",
		Kind:       definitions.KindParent,
		Resource:   "order",
		Mode:       definitions.ModeStandard,
		LeaseTTL:   30 * time.Second,
		ParentRef:  "item",
		KeyBuilder: definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}

	_, err := ResolveAcquirePlan(defs["item"], defs, map[string]string{
		"order_id": "123",
		"item_id":  "line-1",
	})
	if err == nil || err.Error() == "" {
		t.Fatal("expected cyclic parent ref error")
	}
}

func TestResolveAcquirePlanRejectsChildWithoutParentRef(t *testing.T) {
	defs := lineageDefinitions()
	defs["item"] = definitions.LockDefinition{
		ID:            "item",
		Kind:          definitions.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		LeaseTTL:      30 * time.Second,
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
	}

	_, err := ResolveAcquirePlan(defs["item"], defs, map[string]string{
		"order_id": "123",
		"item_id":  "line-1",
	})
	if err == nil || err.Error() == "" {
		t.Fatal("expected missing parent ref error")
	}
}

func TestResolveAcquirePlanRejectsParentRefOnNonChildDefinition(t *testing.T) {
	defs := lineageDefinitions()
	defs["order"] = definitions.LockDefinition{
		ID:         "order",
		Kind:       definitions.KindParent,
		Resource:   "order",
		Mode:       definitions.ModeStandard,
		LeaseTTL:   30 * time.Second,
		ParentRef:  "root",
		KeyBuilder: definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}

	_, err := ResolveAcquirePlan(defs["order"], defs, map[string]string{
		"order_id": "123",
	})
	if err == nil || err.Error() == "" {
		t.Fatal("expected non-child parent ref error")
	}
}

func TestResolveAcquirePlanPropagatesKeyBuilderError(t *testing.T) {
	defs := lineageDefinitions()

	_, err := ResolveAcquirePlan(defs["item"], defs, map[string]string{
		"order_id": "123",
	})
	if err == nil {
		t.Fatal("expected key builder error")
	}
}

func TestResolveAcquirePlanReturnsLeaseIDGenerationError(t *testing.T) {
	restore := swapLeaseIDReader(func([]byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	})
	defer restore()

	_, err := ResolveAcquirePlan(lineageDefinitions()["item"], lineageDefinitions(), map[string]string{
		"order_id": "123",
		"item_id":  "line-1",
	})
	if err == nil || !errors.Is(err, errLeaseIDGenerationFailed) {
		t.Fatalf("expected lease id generation error, got %v", err)
	}
}

func lineageDefinitions() map[string]definitions.LockDefinition {
	return map[string]definitions.LockDefinition{
		"order": {
			ID:         "order",
			Kind:       definitions.KindParent,
			Resource:   "order",
			Mode:       definitions.ModeStandard,
			LeaseTTL:   30 * time.Second,
			KeyBuilder: definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		},
		"item": {
			ID:            "item",
			Kind:          definitions.KindChild,
			Resource:      "item",
			Mode:          definitions.ModeStandard,
			LeaseTTL:      30 * time.Second,
			ParentRef:     "order",
			OverlapPolicy: definitions.OverlapReject,
			KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"}),
		},
	}
}

func swapLeaseIDReader(fn func([]byte) (int, error)) func() {
	previous := leaseIDReader
	leaseIDReader = fn
	return func() {
		leaseIDReader = previous
	}
}
