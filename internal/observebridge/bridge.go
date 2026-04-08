package observebridge

import (
	"context"
	"time"

	"github.com/tuanuet/lockman/lockkit/runtime"
	"github.com/tuanuet/lockman/observe"
)

// Sink is an alias for observe.Sink to allow inspect.Store and stubs.
type Sink = observe.Sink

// Config holds the dependencies for the observability bridge.
type Config struct {
	Store      Sink
	Dispatcher observe.Dispatcher
}

// Bridge connects the inspect store (local state) with the observe dispatcher
// (async event export). It enforces strict once-only semantics: each event is
// applied to the store exactly once and published to the dispatcher exactly once.
type Bridge struct {
	store      Sink
	dispatcher observe.Dispatcher
}

// New constructs a Bridge from the given Config. Either Store or Dispatcher
// (or both) may be nil; the bridge skips nil targets.
func New(cfg Config) *Bridge {
	return &Bridge{
		store:      cfg.Store,
		dispatcher: cfg.Dispatcher,
	}
}

// Shutdown delegates to the dispatcher's Shutdown, respecting the caller's
// context deadline without extending it. If the dispatcher is nil, Shutdown is
// a no-op.
func (b *Bridge) Shutdown(ctx context.Context) error {
	if b.dispatcher == nil {
		return nil
	}
	return b.dispatcher.Shutdown(ctx)
}

// publish applies the event to the local store first, then publishes one async
// copy to the dispatcher. Nil targets are silently skipped.
func (b *Bridge) publish(event observe.Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if b.store != nil {
		if err := b.store.Consume(context.Background(), event); err != nil {
			// Best-effort: store errors are not propagated.
		}
	}
	if b.dispatcher != nil {
		b.dispatcher.Publish(event)
	}
}

var _ runtime.Bridge = (*Bridge)(nil)
