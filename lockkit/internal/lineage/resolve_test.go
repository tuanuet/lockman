package lineage

import (
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
