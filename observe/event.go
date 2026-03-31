package observe

import (
	"context"
	"time"
)

type EventKind int

const (
	EventAcquireStarted EventKind = iota + 1
	EventAcquireSucceeded
	EventAcquireFailed
	EventReleased
	EventContention
	EventOverlap
	EventLeaseLost
	EventRenewalSucceeded
	EventRenewalFailed
	EventShutdownStarted
	EventShutdownCompleted
)

func (k EventKind) String() string {
	switch k {
	case EventAcquireStarted:
		return "acquire_started"
	case EventAcquireSucceeded:
		return "acquire_succeeded"
	case EventAcquireFailed:
		return "acquire_failed"
	case EventReleased:
		return "released"
	case EventContention:
		return "contention"
	case EventOverlap:
		return "overlap"
	case EventLeaseLost:
		return "lease_lost"
	case EventRenewalSucceeded:
		return "renewal_succeeded"
	case EventRenewalFailed:
		return "renewal_failed"
	case EventShutdownStarted:
		return "shutdown_started"
	case EventShutdownCompleted:
		return "shutdown_completed"
	default:
		return ""
	}
}

func (k EventKind) IsValid() bool {
	return k >= EventAcquireStarted && k <= EventShutdownCompleted
}

type Event struct {
	Kind         EventKind
	DefinitionID string
	RequestID    string
	OwnerID      string
	ResourceID   string
	Wait         time.Duration
	Held         time.Duration
	Contention   int
	Success      bool
	Timestamp    time.Time
	Error        error
}

type Sink interface {
	Consume(ctx context.Context, event Event) error
}

type SinkFunc func(ctx context.Context, event Event) error

func (f SinkFunc) Consume(ctx context.Context, event Event) error {
	return f(ctx, event)
}

type Exporter interface {
	Export(ctx context.Context, event Event) error
}

type ExporterFunc func(ctx context.Context, event Event) error

func (f ExporterFunc) Export(ctx context.Context, event Event) error {
	return f(ctx, event)
}
