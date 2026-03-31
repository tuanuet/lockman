module github.com/tuanuet/lockman/examples

go 1.22

require (
	github.com/alicebob/miniredis/v2 v2.37.0
	github.com/jackc/pgx/v5 v5.6.0
	github.com/redis/go-redis/v9 v9.18.0
	github.com/tuanuet/lockman v0.0.0
	github.com/tuanuet/lockman/backend/redis v1.0.0
	github.com/tuanuet/lockman/guard/postgres v1.0.0
	github.com/tuanuet/lockman/idempotency/redis v1.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.17.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/text v0.14.0 // indirect
)

replace github.com/tuanuet/lockman => ../

replace github.com/tuanuet/lockman/backend/redis => ../backend/redis

replace github.com/tuanuet/lockman/guard/postgres => ../guard/postgres

replace github.com/tuanuet/lockman/idempotency/redis => ../idempotency/redis
