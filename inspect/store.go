package inspect

import (
	"context"
	"time"

	"github.com/tuanuet/lockman/observe"
)

type RuntimeLock struct {
	DefinitionID string
	ResourceID   string
	OwnerID      string
	AcquiredAt   time.Time
}

type WorkerClaim struct {
	DefinitionID string
	ResourceID   string
	WorkerID     string
	ClaimedAt    time.Time
}

type Renewal struct {
	DefinitionID  string
	ResourceID    string
	OwnerID       string
	RenewedAt     time.Time
	NewExpiration time.Time
}

type Shutdown struct {
	DefinitionID string
	OwnerID      string
	StartedAt    time.Time
	CompletedAt  *time.Time
}

type PipelineState struct {
	Status     string
	QueueDepth int
	Errors     int
}

type Snapshot struct {
	RuntimeLocks []RuntimeLock
	WorkerClaims []WorkerClaim
	Renewals     []Renewal
	Shutdowns    []Shutdown
	Pipeline     PipelineState
}

type Event struct {
	Kind         observe.EventKind
	DefinitionID string
	ResourceID   string
	OwnerID      string
	Timestamp    time.Time
}

type QueryFilter struct {
	LockID      string
	ResourceKey string
	OwnerID     string
	Kind        observe.EventKind
	Since       time.Time
	Until       time.Time
}

type StoreOption func(*store)

func WithHistoryLimit(limit int) StoreOption {
	return func(s *store) {
		s.historyLimit = limit
	}
}

type store struct {
	runtimeLocks map[string]RuntimeLock
	workerClaims map[string]WorkerClaim
	renewals     []Renewal
	shutdowns    []Shutdown
	pipeline     PipelineState
	history      []Event
	historyLimit int
	subscribers  []chan Event
}

func NewStore(opts ...StoreOption) *store {
	s := &store{
		runtimeLocks: make(map[string]RuntimeLock),
		workerClaims: make(map[string]WorkerClaim),
		historyLimit: 100,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *store) Consume(ctx context.Context, event observe.Event) error {
	e := Event{
		Kind:         event.Kind,
		DefinitionID: event.DefinitionID,
		ResourceID:   event.ResourceID,
		OwnerID:      event.OwnerID,
		Timestamp:    event.Timestamp,
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	s.addToHistory(e)
	s.applyEvent(e)
	s.notifySubscribers(e)

	return nil
}

func (s *store) addToHistory(e Event) {
	s.history = append(s.history, e)
	if s.historyLimit > 0 && len(s.history) > s.historyLimit {
		s.history = s.history[len(s.history)-s.historyLimit:]
	}
}

func (s *store) applyEvent(e Event) {
	key := e.ResourceID + ":" + e.DefinitionID

	switch e.Kind {
	case observe.EventAcquireSucceeded:
		s.runtimeLocks[key] = RuntimeLock{
			DefinitionID: e.DefinitionID,
			ResourceID:   e.ResourceID,
			OwnerID:      e.OwnerID,
			AcquiredAt:   e.Timestamp,
		}
		s.workerClaims[key] = WorkerClaim{
			DefinitionID: e.DefinitionID,
			ResourceID:   e.ResourceID,
			WorkerID:     e.OwnerID,
			ClaimedAt:    e.Timestamp,
		}

	case observe.EventReleased:
		delete(s.runtimeLocks, key)

	case observe.EventRenewalSucceeded:
		s.renewals = append(s.renewals, Renewal{
			DefinitionID:  e.DefinitionID,
			ResourceID:    e.ResourceID,
			OwnerID:       e.OwnerID,
			RenewedAt:     e.Timestamp,
			NewExpiration: e.Timestamp.Add(30 * time.Second),
		})

	case observe.EventShutdownStarted:
		s.shutdowns = append(s.shutdowns, Shutdown{
			DefinitionID: e.DefinitionID,
			OwnerID:      e.OwnerID,
			StartedAt:    e.Timestamp,
		})

	case observe.EventShutdownCompleted:
		s.shutdowns = append(s.shutdowns, Shutdown{
			DefinitionID: e.DefinitionID,
			OwnerID:      e.OwnerID,
			StartedAt:    time.Time{},
			CompletedAt:  &e.Timestamp,
		})
	}
}

func (s *store) notifySubscribers(e Event) {
	for _, sub := range s.subscribers {
		select {
		case sub <- e:
		default:
		}
	}
}

func (s *store) Snapshot() Snapshot {
	locks := make([]RuntimeLock, 0, len(s.runtimeLocks))
	for _, l := range s.runtimeLocks {
		locks = append(locks, l)
	}

	claims := make([]WorkerClaim, 0, len(s.workerClaims))
	for _, c := range s.workerClaims {
		claims = append(claims, c)
	}

	return Snapshot{
		RuntimeLocks: locks,
		WorkerClaims: claims,
		Renewals:     s.renewals,
		Shutdowns:    s.shutdowns,
		Pipeline:     s.pipeline,
	}
}

func (s *store) RecentEvents() []Event {
	result := make([]Event, len(s.history))
	copy(result, s.history)
	return result
}

func (s *store) Query(filter QueryFilter) []Event {
	var result []Event
	for _, e := range s.history {
		if filter.LockID != "" && e.DefinitionID != filter.LockID {
			continue
		}
		if filter.ResourceKey != "" && e.ResourceID != filter.ResourceKey {
			continue
		}
		if filter.OwnerID != "" && e.OwnerID != filter.OwnerID {
			continue
		}
		if filter.Kind != 0 && e.Kind != filter.Kind {
			continue
		}
		if !filter.Since.IsZero() && e.Timestamp.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && e.Timestamp.After(filter.Until) {
			continue
		}
		result = append(result, e)
	}
	return result
}

func (s *store) Subscribe(fn func(observe.Event) error) {
	ch := make(chan Event, 10)
	s.subscribers = append(s.subscribers, ch)

	go func() {
		for e := range ch {
			fn(observe.Event{
				Kind:         e.Kind,
				DefinitionID: e.DefinitionID,
				ResourceID:   e.ResourceID,
				OwnerID:      e.OwnerID,
				Timestamp:    e.Timestamp,
			})
		}
	}()
}

func (s *store) UpdatePipelineState(state PipelineState) {
	s.pipeline = state
}

func (s *store) Events() <-chan Event {
	ch := make(chan Event, len(s.history))
	for _, e := range s.history {
		ch <- e
	}
	close(ch)
	return ch
}
