package memory

import (
	"context"
	"errors"
	"hash/fnv"
	"sync"
	"time"

	"github.com/tuanuet/lockman/idempotency"
)

const shardCount = 16

var errInvalidTTL = errors.New("idempotency: invalid ttl")

type memoryShard struct {
	mu      sync.Mutex
	records map[string]idempotency.Record
}

// Store is an in-memory idempotency store intended for tests and local runs.
type Store struct {
	shards [shardCount]memoryShard
	now    func() time.Time
}

// NewStore returns a ready-to-use in-memory store.
func NewStore() *Store {
	return NewStoreWithNow(time.Now)
}

// NewStoreWithNow returns an in-memory store with a custom clock.
func NewStoreWithNow(now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	s := &Store{now: now}
	for i := range s.shards {
		s.shards[i].records = make(map[string]idempotency.Record)
	}
	return s
}

func (s *Store) shard(key string) *memoryShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return &s.shards[h.Sum32()%shardCount]
}

func (s *Store) Get(ctx context.Context, key string) (idempotency.Record, error) {
	if err := ctx.Err(); err != nil {
		return idempotency.Record{}, err
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
		return idempotency.Record{
			Key:    key,
			Status: idempotency.StatusMissing,
		}, nil
	}

	return record, nil
}

func (s *Store) Begin(ctx context.Context, key string, input idempotency.BeginInput) (idempotency.BeginResult, error) {
	if err := ctx.Err(); err != nil {
		return idempotency.BeginResult{}, err
	}
	if input.TTL <= 0 {
		return idempotency.BeginResult{}, errInvalidTTL
	}

	sh := s.shard(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	now := s.now()

	if record, ok := sh.records[key]; ok {
		if !isExpired(record.ExpiresAt, now) {
			return idempotency.BeginResult{
				Record:    record,
				Acquired:  false,
				Duplicate: true,
			}, nil
		}
		delete(sh.records, key)
	}

	record := idempotency.Record{
		Key:           key,
		Status:        idempotency.StatusInProgress,
		OwnerID:       input.OwnerID,
		MessageID:     input.MessageID,
		ConsumerGroup: input.ConsumerGroup,
		Attempt:       input.Attempt,
		UpdatedAt:     now,
		ExpiresAt:     now.Add(input.TTL),
	}
	sh.records[key] = record

	return idempotency.BeginResult{
		Record:    record,
		Acquired:  true,
		Duplicate: false,
	}, nil
}

func (s *Store) Complete(ctx context.Context, key string, input idempotency.CompleteInput) error {
	return s.setTerminalStatus(ctx, key, input.OwnerID, input.MessageID, input.TTL, idempotency.StatusCompleted)
}

func (s *Store) Fail(ctx context.Context, key string, input idempotency.FailInput) error {
	return s.setTerminalStatus(ctx, key, input.OwnerID, input.MessageID, input.TTL, idempotency.StatusFailed)
}

func (s *Store) setTerminalStatus(ctx context.Context, key, ownerID, messageID string, ttl time.Duration, status idempotency.Status) error {
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

	record := idempotency.Record{
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

var _ idempotency.Store = (*Store)(nil)
