// Deprecated: Import "lockman/idempotency" instead.
//
// This file is a compatibility shim for older lockkit import paths. The in-memory
// store implementation now lives at lockman/idempotency.
package idempotency

import (
	"time"

	promoted "lockman/idempotency"
)

// Deprecated: use lockman/idempotency.MemoryStore.
type MemoryStore = promoted.MemoryStore

// Deprecated: use lockman/idempotency.NewMemoryStore.
func NewMemoryStore() *MemoryStore {
	return promoted.NewMemoryStore()
}

// Deprecated: use lockman/idempotency.NewMemoryStoreWithNow.
func NewMemoryStoreWithNow(now func() time.Time) *MemoryStore {
	return promoted.NewMemoryStoreWithNow(now)
}
