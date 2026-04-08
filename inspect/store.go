package inspect

import (
	"context"
	"sync"

	"github.com/tuanuet/lockman/observe"
)

const defaultHistoryLimit = 500

// Store is an in-memory hot-path inspect store. It implements observe.Sink.
type Store struct {
	mu sync.RWMutex

	// Runtime state materialised from events.
	runtimeLocks map[string]RuntimeLockInfo // key: definitionID:resourceID:ownerID
	workerClaims map[string]WorkerClaimInfo
	renewals     map[string]RenewalInfo
	shutdown     ShutdownInfo
	pipeline     PipelineState

	// Ring buffer for recent events.
	history     []observe.Event
	historyCap  int
	historyHead int // next write position
	historyLen  int // number of valid entries

	// Subscribers.
	subMu sync.RWMutex
	subs  map[chan<- observe.Event]struct{}
}

// Option configures a Store.
type Option func(*Store)

// WithHistoryLimit sets the ring buffer capacity.
func WithHistoryLimit(n int) Option {
	return func(s *Store) {
		if n > 0 {
			s.historyCap = n
			s.history = make([]observe.Event, n)
		}
	}
}

// NewStore returns a ready-to-use Store.
func NewStore(opts ...Option) *Store {
	s := &Store{
		runtimeLocks: make(map[string]RuntimeLockInfo),
		workerClaims: make(map[string]WorkerClaimInfo),
		renewals:     make(map[string]RenewalInfo),
		historyCap:   defaultHistoryLimit,
		history:      make([]observe.Event, defaultHistoryLimit),
		subs:         make(map[chan<- observe.Event]struct{}),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Consume implements observe.Sink. It materialises runtime state and appends to the ring buffer.
func (s *Store) Consume(_ context.Context, event observe.Event) error {
	s.mu.Lock()
	s.applyEventLocked(event)
	s.appendToRingLocked(event)
	s.mu.Unlock()

	s.notifySubscribers(event)
	return nil
}

// applyEventLocked updates runtime state. Caller must hold s.mu.
func (s *Store) applyEventLocked(e observe.Event) {
	key := lockKey(e.DefinitionID, e.ResourceID, e.OwnerID)

	switch e.Kind {
	case observe.EventAcquireSucceeded:
		s.runtimeLocks[key] = RuntimeLockInfo{
			DefinitionID: e.DefinitionID,
			ResourceID:   e.ResourceID,
			OwnerID:      e.OwnerID,
			AcquiredAt:   e.Timestamp,
		}
		delete(s.workerClaims, key)
	case observe.EventAcquireFailed:
		delete(s.workerClaims, key)
	case observe.EventAcquireStarted:
		s.workerClaims[key] = WorkerClaimInfo{
			DefinitionID: e.DefinitionID,
			ResourceID:   e.ResourceID,
			OwnerID:      e.OwnerID,
			ClaimedAt:    e.Timestamp,
		}
	case observe.EventReleased, observe.EventLeaseLost:
		delete(s.runtimeLocks, key)
	case observe.EventRenewalSucceeded:
		s.renewals[key] = RenewalInfo{
			DefinitionID: e.DefinitionID,
			ResourceID:   e.ResourceID,
			OwnerID:      e.OwnerID,
			LastRenewed:  e.Timestamp,
		}
	case observe.EventShutdownStarted:
		s.shutdown.Started = true
	case observe.EventShutdownCompleted:
		s.shutdown.Completed = true
	}
}

// appendToRingLocked writes an event into the ring buffer. Caller must hold s.mu.
func (s *Store) appendToRingLocked(e observe.Event) {
	s.history[s.historyHead] = e
	s.historyHead = (s.historyHead + 1) % s.historyCap
	if s.historyLen < s.historyCap {
		s.historyLen++
	}
}

// notifySubscribers sends the event to all subscribers (non-blocking).
func (s *Store) notifySubscribers(e observe.Event) {
	s.subMu.RLock()
	defer s.subMu.RUnlock()
	for ch := range s.subs {
		select {
		case ch <- e:
		default:
			// slow subscriber – drop to avoid blocking the hot path
		}
	}
}

// Snapshot returns a point-in-time view of the store.
func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	locks := make([]RuntimeLockInfo, 0, len(s.runtimeLocks))
	for _, v := range s.runtimeLocks {
		locks = append(locks, v)
	}

	claims := make([]WorkerClaimInfo, 0, len(s.workerClaims))
	for _, v := range s.workerClaims {
		claims = append(claims, v)
	}

	renewals := make([]RenewalInfo, 0, len(s.renewals))
	for _, v := range s.renewals {
		renewals = append(renewals, v)
	}

	return Snapshot{
		RuntimeLocks: locks,
		WorkerClaims: claims,
		Renewals:     renewals,
		Shutdown:     s.shutdown,
		Pipeline:     s.pipeline,
	}
}

// RecentEvents returns up to limit events in chronological order.
func (s *Store) RecentEvents(limit int) []observe.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > s.historyLen {
		limit = s.historyLen
	}

	out := make([]observe.Event, 0, limit)

	// Find the oldest entry in the ring.
	start := 0
	if s.historyLen == s.historyCap {
		start = s.historyHead
	}
	for i := 0; i < limit; i++ {
		idx := (start + i) % s.historyCap
		out = append(out, s.history[idx])
	}
	return out
}

// Query filters the stored event history by the given options.
func (s *Store) Query(opts QueryOptions) []observe.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := s.ringEventsLocked()

	var out []observe.Event
	for _, e := range all {
		if opts.DefinitionID != "" && e.DefinitionID != opts.DefinitionID {
			continue
		}
		if opts.ResourceID != "" && e.ResourceID != opts.ResourceID {
			continue
		}
		if opts.OwnerID != "" && e.OwnerID != opts.OwnerID {
			continue
		}
		if opts.Kind != 0 && e.Kind != opts.Kind {
			continue
		}
		if !opts.Since.IsZero() && e.Timestamp.Before(opts.Since) {
			continue
		}
		if !opts.Until.IsZero() && e.Timestamp.After(opts.Until) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// ringEventsLocked returns all valid events from the ring buffer in chronological order. Caller must hold s.mu.
func (s *Store) ringEventsLocked() []observe.Event {
	out := make([]observe.Event, 0, s.historyLen)
	start := 0
	if s.historyLen == s.historyCap {
		start = s.historyHead
	}
	for i := 0; i < s.historyLen; i++ {
		idx := (start + i) % s.historyCap
		out = append(out, s.history[idx])
	}
	return out
}

// Subscribe registers ch for real-time event delivery. Returns an unsubscribe function.
func (s *Store) Subscribe(ch chan<- observe.Event) func() {
	s.subMu.Lock()
	s.subs[ch] = struct{}{}
	s.subMu.Unlock()

	return func() {
		s.subMu.Lock()
		delete(s.subs, ch)
		s.subMu.Unlock()
	}
}

// UpdatePipelineState sets dispatcher-level counters on the store.
func (s *Store) UpdatePipelineState(ps PipelineState) {
	s.mu.Lock()
	s.pipeline = ps
	s.mu.Unlock()
}

func lockKey(definitionID, resourceID, ownerID string) string {
	return definitionID + ":" + resourceID + ":" + ownerID
}
