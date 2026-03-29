package idempotency

import (
	"context"
	"errors"
	"sync"
	"time"
)

var errInvalidTTL = errors.New("idempotency: invalid ttl")

// MemoryStore is an in-memory idempotency store intended for tests and local runs.
type MemoryStore struct {
	mu      sync.Mutex
	records map[string]Record
	now     func() time.Time
}

func NewMemoryStore() *MemoryStore {
	return NewMemoryStoreWithNow(time.Now)
}

func NewMemoryStoreWithNow(now func() time.Time) *MemoryStore {
	if now == nil {
		now = time.Now
	}
	return &MemoryStore{
		records: make(map[string]Record),
		now:     now,
	}
}

func (s *MemoryStore) Get(ctx context.Context, key string) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()

	record, ok := s.records[key]
	if !ok || isExpired(record.ExpiresAt, now) {
		if ok {
			delete(s.records, key)
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

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()

	if record, ok := s.records[key]; ok {
		if !isExpired(record.ExpiresAt, now) {
			return BeginResult{
				Record:    record,
				Acquired:  false,
				Duplicate: true,
			}, nil
		}
		delete(s.records, key)
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
	s.records[key] = record

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

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()

	record := Record{
		Key:       key,
		Status:    status,
		OwnerID:   ownerID,
		MessageID: messageID,
		UpdatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if existing, ok := s.records[key]; ok && !isExpired(existing.ExpiresAt, now) {
		record.OwnerID = existing.OwnerID
		record.MessageID = existing.MessageID
		record.ConsumerGroup = existing.ConsumerGroup
		record.Attempt = existing.Attempt
	}

	s.records[key] = record
	return nil
}

func isExpired(expiresAt, now time.Time) bool {
	return !expiresAt.After(now)
}

var _ Store = (*MemoryStore)(nil)

