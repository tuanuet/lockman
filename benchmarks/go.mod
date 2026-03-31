module github.com/tuanuet/lockman/benchmarks

go 1.22

require (
	github.com/alicebob/miniredis/v2 v2.37.0
	github.com/redis/go-redis/v9 v9.18.0
	github.com/tuanuet/lockman v0.0.0
	github.com/tuanuet/lockman/backend/redis v1.0.0
	github.com/tuanuet/lockman/idempotency/redis v1.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/tuanuet/lockman => ../

replace github.com/tuanuet/lockman/backend/redis => ../backend/redis

replace github.com/tuanuet/lockman/idempotency/redis => ../idempotency/redis
