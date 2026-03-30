// Package redis is a temporary compatibility shim.
//
// Deprecated: new code should import the extracted adapter module at
// `lockman/idempotency/redis`.
package redis

import (
	goredis "github.com/redis/go-redis/v9"

	"lockman/idempotency"
	idredis "lockman/idempotency/redis"
)

// Store is a type alias to the extracted adapter store.
//
// Deprecated: new code should import `lockman/idempotency/redis` directly.
type Store = idredis.Store

// New creates a Redis idempotency store compatible with lockman.WithIdempotency(...).
//
// Deprecated: use `lockman/idempotency/redis.New` directly.
func New(client goredis.UniversalClient, keyPrefix string) idempotency.Store {
	return idredis.New(client, keyPrefix)
}

// NewStore constructs a Redis-backed idempotency store.
//
// Deprecated: use `lockman/idempotency/redis.NewStore` directly.
func NewStore(client goredis.UniversalClient, keyPrefix string) *Store {
	return idredis.NewStore(client, keyPrefix)
}
