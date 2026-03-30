package lockman_test

import (
	"errors"
	"testing"

	"github.com/tuanuet/lockman"
)

func TestErrRegistryRequiredIsStableSentinel(t *testing.T) {
	err := lockman.ErrRegistryRequired
	if !errors.Is(err, lockman.ErrRegistryRequired) {
		t.Fatal("expected ErrRegistryRequired to remain a stable sentinel")
	}
}

func TestNewPlaceholderReturnsRegistryRequired(t *testing.T) {
	cases := []struct {
		name string
		opts []lockman.ClientOption
	}{
		{name: "without options"},
		{
			name: "with identity option",
			opts: []lockman.ClientOption{
				lockman.WithIdentity(lockman.Identity{OwnerID: "owner-1"}),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := lockman.New(tc.opts...)
			if client != nil {
				t.Fatalf("expected nil client, got %#v", client)
			}
			if !errors.Is(err, lockman.ErrRegistryRequired) {
				t.Fatalf("expected ErrRegistryRequired, got %v", err)
			}
		})
	}
}
