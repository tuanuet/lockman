package idempotency

import (
	"context"
	"errors"
	"hash/fnv"
	"sync"
	"time"
)

const shardCount = 16

var errInvalidTTL = errors.New("idempotency: invalid ttl")

type memoryShard struct {
	mu      sync.Mutex
	records map[string]Record
}

// MemoryStore is an in-memory idempotency store intended for tests and local runs.
type MemoryStore struct {
	shards [shardCount]memoryShard
	now    func() time.Time
}

func NewMemoryStore() *MemoryStore {
	return NewMemoryStoreWithNow(time.Now)
}

func NewMemoryStoreWithNow(now func() time.Time) *MemoryStore {
	if now == nil {
		now = time.Now
	}
	s := &MemoryStore{now: now}
	for i := range s.shards {
		s.shards[i].records = make(map[string]Record)
	}
	return s
}

func (s *MemoryStore) shard(key string) *memoryShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return &s.shards[h.Sum32()%shardCount]
}

func (s *MemoryStore) Get(ctx context.Context, key string) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}

	sh := s.shard(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	now := s.now()

	record, ok := sh.records[key]
	if !ok || isExpired(record.ExpiresAt, now) {
		if ok {
			delete(sh.records, key)
		}
		return Record{
			Key:    key,
			Status: StatusMissing,
		}, nil
	}

	return record, nil
}

func (s *MemoryStore) Begin(ctx context.Context, key string, input BeginInput) (BeginResult, error) {
	if err := ctx.Err(); err != nil {
		return BeginResult{}, err
	}
	if input.TTL <= 0 {
		return BeginResult{}, errInvalidTTL
	}

	sh := s.shard(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	now := s.now()

	if record, ok := sh.records[key]; ok {
		if !isExpired(record.ExpiresAt, now) {
			return BeginResult{
				Record:    record,
				Acquired:  false,
				Duplicate: true,
			}, nil
		}
		delete(sh.records, key)
	}

	record := Record{
		Key:           key,
		Status:        StatusInProgress,
		OwnerID:       input.OwnerID,
		MessageID:     input.MessageID,
		ConsumerGroup: input.ConsumerGroup,
		Attempt:       input.Attempt,
		UpdatedAt:     now,
		ExpiresAt:     now.Add(input.TTL),
	}
	sh.records[key] = record

	return BeginResult{
		Record:    record,
		Acquired:  true,
		Duplicate: false,
	}, nil
}

func (s *MemoryStore) Complete(ctx context.Context, key string, input CompleteInput) error {
	return s.setTerminalStatus(ctx, key, input.OwnerID, input.MessageID, input.TTL, StatusCompleted)
}

func (s *MemoryStore) Fail(ctx context.Context, key string, input FailInput) error {
	return s.setTerminalStatus(ctx, key, input.OwnerID, input.MessageID, input.TTL, StatusFailed)
}

func (s *MemoryStore) setTerminalStatus(ctx context.Context, key, ownerID, messageID string, ttl time.Duration, status Status) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ttl <= 0 {
		return errInvalidTTL
	}

	sh := s.shard(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	now := s.now()

	record := Record{
		Key:       key,
		Status:    status,
		OwnerID:   ownerID,
		MessageID: messageID,
		UpdatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if existing, ok := sh.records[key]; ok && !isExpired(existing.ExpiresAt, now) {
		record.OwnerID = existing.OwnerID
		record.MessageID = existing.MessageID
		record.ConsumerGroup = existing.ConsumerGroup
		record.Attempt = existing.Attempt
	}

	sh.records[key] = record
	return nil
}

func isExpired(expiresAt, now time.Time) bool {
	return !expiresAt.After(now)
}

var _ Store = (*MemoryStore)(nil)
