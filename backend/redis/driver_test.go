package redis

import "testing"

func TestCacheDefinitionIDsBuildsConsistentLeaseKey(t *testing.T) {
	drv := &Driver{keyPrefix: "lockman:lease"}
	uncached := drv.buildLeaseKey("order.standard", "order:12345")

	drv.CacheDefinitionIDs([]string{"order.standard"})
	cached := drv.buildLeaseKey("order.standard", "order:12345")

	if cached != uncached {
		t.Fatalf("expected cached key %q to match uncached key %q", cached, uncached)
	}
}

func TestCacheDefinitionIDsBuildsConsistentStrictAndLineageKeys(t *testing.T) {
	drv := &Driver{keyPrefix: "lockman:lease"}
	uncachedFence := drv.buildStrictFenceCounterKey("order.standard", "order:12345")
	uncachedToken := drv.buildStrictTokenKey("order.standard", "order:12345")
	uncachedLineage := drv.buildLineageKey("order.standard", "order:12345")

	drv.CacheDefinitionIDs([]string{"order.standard"})
	cachedFence := drv.buildStrictFenceCounterKey("order.standard", "order:12345")
	cachedToken := drv.buildStrictTokenKey("order.standard", "order:12345")
	cachedLineage := drv.buildLineageKey("order.standard", "order:12345")

	if cachedFence != uncachedFence {
		t.Fatalf("expected cached fence key %q to match uncached key %q", cachedFence, uncachedFence)
	}
	if cachedToken != uncachedToken {
		t.Fatalf("expected cached token key %q to match uncached key %q", cachedToken, uncachedToken)
	}
	if cachedLineage != uncachedLineage {
		t.Fatalf("expected cached lineage key %q to match uncached key %q", cachedLineage, uncachedLineage)
	}
}
