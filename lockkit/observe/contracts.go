package observe

import (
	"context"
	"time"
)

type Recorder interface {
	RecordAcquire(ctx context.Context, definitionID string, wait time.Duration, success bool)
	RecordContention(ctx context.Context, definitionID string)
	RecordTimeout(ctx context.Context, definitionID string)
	RecordActiveLocks(ctx context.Context, definitionID string, count int)
	RecordRelease(ctx context.Context, definitionID string, held time.Duration)
	RecordPresenceCheck(ctx context.Context, definitionID string, duration time.Duration)
}

type noopRecorder struct{}

func (noopRecorder) RecordAcquire(ctx context.Context, definitionID string, wait time.Duration, success bool) {
	_ = ctx
	_ = definitionID
	_ = wait
	_ = success
}

func (noopRecorder) RecordContention(ctx context.Context, definitionID string) {
	_ = ctx
	_ = definitionID
}

func (noopRecorder) RecordTimeout(ctx context.Context, definitionID string) {
	_ = ctx
	_ = definitionID
}

func (noopRecorder) RecordActiveLocks(ctx context.Context, definitionID string, count int) {
	_ = ctx
	_ = definitionID
	_ = count
}

func (noopRecorder) RecordRelease(ctx context.Context, definitionID string, held time.Duration) {
	_ = ctx
	_ = definitionID
	_ = held
}

func (noopRecorder) RecordPresenceCheck(ctx context.Context, definitionID string, duration time.Duration) {
	_ = ctx
	_ = definitionID
	_ = duration
}

func NewNoopRecorder() Recorder {
	return noopRecorder{}
}
