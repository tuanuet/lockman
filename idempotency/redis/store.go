package redis

import (
	goredis "github.com/redis/go-redis/v9"

	"lockman/lockkit/idempotency"
	redisstore "lockman/lockkit/idempotency/redis"
)

// New creates a Redis idempotency store compatible with lockman.WithIdempotency(...).
func New(client goredis.UniversalClient, keyPrefix string) idempotency.Store {
	return redisstore.NewStore(client, keyPrefix)
}
