// Package redis provides a transitional Redis-backed implementation of the
// top-level lockman/idempotency.Store.
//
// Note: this package currently delegates to the legacy lockkit Redis store
// implementation. It will be replaced by the extracted adapter module in a
// later task.
package redis

import (
	goredis "github.com/redis/go-redis/v9"

	"lockman/idempotency"
	redisstore "lockman/lockkit/idempotency/redis"
)

// New creates a Redis idempotency store compatible with lockman.WithIdempotency(...).
// Passing an empty keyPrefix uses lockman's default Redis idempotency namespace.
func New(client goredis.UniversalClient, keyPrefix string) idempotency.Store {
	return redisstore.NewStore(client, keyPrefix)
}
