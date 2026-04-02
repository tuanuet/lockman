package redis

import "testing"

func BenchmarkBuildLeaseKey(b *testing.B) {
	drv := &Driver{
		keyPrefix: "lockman:lease",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = drv.buildLeaseKey("order.standard", "order:12345")
	}
}

func BenchmarkBuildLeaseKeyWithCache(b *testing.B) {
	drv := &Driver{
		keyPrefix:  "lockman:lease",
		encodedIDs: map[string]string{},
	}
	drv.cacheDefinitionID("order.standard")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = drv.buildLeaseKey("order.standard", "order:12345")
	}
}
