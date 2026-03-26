package observe

import (
	"context"
	"time"
)

// Recorder captures lock lifecycle events so observers can monitor system behavior.
type Recorder interface {
	RecordAcquire(ctx context.Context, definitionID string, wait time.Duration, success bool)
	RecordContention(ctx context.Context, definitionID string)
	RecordTimeout(ctx context.Context, definitionID string)
	RecordActiveLocks(ctx context.Context, definitionID string, count int)
	RecordRelease(ctx context.Context, definitionID string, held time.Duration)
	RecordPresenceCheck(ctx context.Context, definitionID string, duration time.Duration)
}

type noopRecorder struct{}

func (noopRecorder) RecordAcquire(context.Context, string, time.Duration, bool) {}

func (noopRecorder) RecordContention(context.Context, string) {}

func (noopRecorder) RecordTimeout(context.Context, string) {}

func (noopRecorder) RecordActiveLocks(context.Context, string, int) {}

func (noopRecorder) RecordRelease(context.Context, string, time.Duration) {}

func (noopRecorder) RecordPresenceCheck(context.Context, string, time.Duration) {}

// NewNoopRecorder produces a Recorder implementation that drops all events.
func NewNoopRecorder() Recorder {
	return noopRecorder{}
}

var _ Recorder = noopRecorder{}
