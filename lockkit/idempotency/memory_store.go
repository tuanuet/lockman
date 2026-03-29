package idempotency

import (
	"time"

	promoted "lockman/idempotency"
)

type MemoryStore = promoted.MemoryStore

func NewMemoryStore() *MemoryStore {
	return promoted.NewMemoryStore()
}

func NewMemoryStoreWithNow(now func() time.Time) *MemoryStore {
	return promoted.NewMemoryStoreWithNow(now)
}

