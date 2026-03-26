package idempotency

import (
	"context"
	"sync"
	"time"
)

type MemoryStore struct {
	mu      sync.Mutex
	records map[string]Record
	now     func() time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		records: make(map[string]Record),
		now:     time.Now,
	}
}

func (s *MemoryStore) Get(ctx context.Context, key string) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}

	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.records[key]
	if !ok || record.ExpiresAt.Before(now) {
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

	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if record, ok := s.records[key]; ok {
		if !record.ExpiresAt.Before(now) {
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

	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	record := Record{
		Key:       key,
		Status:    status,
		OwnerID:   ownerID,
		MessageID: messageID,
		UpdatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if existing, ok := s.records[key]; ok && !existing.ExpiresAt.Before(now) {
		record.ConsumerGroup = existing.ConsumerGroup
		record.Attempt = existing.Attempt
		if record.OwnerID == "" {
			record.OwnerID = existing.OwnerID
		}
		if record.MessageID == "" {
			record.MessageID = existing.MessageID
		}
	}

	s.records[key] = record
	return nil
}

var _ Store = (*MemoryStore)(nil)
