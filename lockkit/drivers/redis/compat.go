// Package redis is a temporary compatibility shim.
//
// Deprecated: new code should import the extracted adapter module at `lockman/redis`.
package redis

import (
	goredis "github.com/redis/go-redis/v9"

	lockredis "lockman/redis"
)

// Driver is a type alias to the extracted adapter driver.
type Driver = lockredis.Driver

// NewDriver constructs a Redis-backed lock driver.
//
// Deprecated: use `lockman/redis.NewDriver` (or `lockman/redis.New`) directly.
func NewDriver(client goredis.UniversalClient, keyPrefix string) *Driver {
	return lockredis.NewDriver(client, keyPrefix)
}

