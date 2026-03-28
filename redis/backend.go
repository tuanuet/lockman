package redis

import (
	goredis "github.com/redis/go-redis/v9"

	"lockman/lockkit/drivers"
	lockredis "lockman/lockkit/drivers/redis"
)

// New creates a Redis backend compatible with lockman.WithBackend(...).
// Passing an empty keyPrefix uses lockman's default Redis lease namespace.
func New(client goredis.UniversalClient, keyPrefix string) drivers.Driver {
	return lockredis.NewDriver(client, keyPrefix)
}
