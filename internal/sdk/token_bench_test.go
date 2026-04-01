package sdk

import "testing"

func BenchmarkEncodeHoldToken(b *testing.B) {
	cases := []struct {
		name string
		keys []string
	}{
		{name: "single_key", keys: []string{"order:123"}},
		{name: "three_keys", keys: []string{"contract:abc", "order:xyz", "user:999"}},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				token, err := EncodeHoldToken(tc.keys, "bench-owner")
				if err != nil {
					b.Fatalf("EncodeHoldToken returned error: %v", err)
				}
				if token == "" {
					b.Fatal("expected non-empty token")
				}
			}
		})
	}
}

func BenchmarkDecodeHoldToken(b *testing.B) {
	cases := []struct {
		name string
		keys []string
	}{
		{name: "single_key", keys: []string{"order:123"}},
		{name: "three_keys", keys: []string{"contract:abc", "order:xyz", "user:999"}},
	}

	for _, tc := range cases {
		token, err := EncodeHoldToken(tc.keys, "bench-owner")
		if err != nil {
			b.Fatalf("EncodeHoldToken returned error: %v", err)
		}

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				keys, ownerID, err := DecodeHoldToken(token)
				if err != nil {
					b.Fatalf("DecodeHoldToken returned error: %v", err)
				}
				if len(keys) != len(tc.keys) {
					b.Fatalf("expected %d keys, got %d", len(tc.keys), len(keys))
				}
				if ownerID != "bench-owner" {
					b.Fatalf("expected owner %q, got %q", "bench-owner", ownerID)
				}
			}
		})
	}
}
