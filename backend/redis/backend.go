package redis

import (
	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman/backend"
)

// New creates a Redis backend compatible with lockman.WithBackend(...).
// Passing an empty keyPrefix uses lockman's default Redis lease namespace.
func New(client goredis.UniversalClient, keyPrefix string) backend.Driver {
	return NewDriver(client, keyPrefix)
}
