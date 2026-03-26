package idempotency

import (
	"context"
	"time"
)

type Status string

const (
	StatusMissing    Status = "missing"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

type Record struct {
	Key           string
	Status        Status
	OwnerID       string
	MessageID     string
	ConsumerGroup string
	Attempt       int
	UpdatedAt     time.Time
	ExpiresAt     time.Time
}

type BeginInput struct {
	OwnerID       string
	MessageID     string
	ConsumerGroup string
	Attempt       int
	TTL           time.Duration
}

type BeginResult struct {
	Record    Record
	Acquired  bool
	Duplicate bool
}

type CompleteInput struct {
	OwnerID   string
	MessageID string
	TTL       time.Duration
}

type FailInput struct {
	OwnerID   string
	MessageID string
	TTL       time.Duration
}

type Store interface {
	Get(ctx context.Context, key string) (Record, error)
	Begin(ctx context.Context, key string, input BeginInput) (BeginResult, error)
	Complete(ctx context.Context, key string, input CompleteInput) error
	Fail(ctx context.Context, key string, input FailInput) error
}
