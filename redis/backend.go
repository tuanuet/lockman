package redis

import (
	goredis "github.com/redis/go-redis/v9"

	"lockman/backend"
	lockredis "lockman/lockkit/drivers/redis"
)

// New creates a Redis backend compatible with lockman.WithBackend(...).
// Passing an empty keyPrefix uses lockman's default Redis lease namespace.
//
// This package is a temporary bridge to the internal Redis driver while the adapter
// extraction refactor is in progress. New adapter code should prefer the extracted
// adapter module once it lands, rather than depending on lockkit internals.
func New(client goredis.UniversalClient, keyPrefix string) backend.Driver {
	return lockredis.NewDriver(client, keyPrefix)
}
