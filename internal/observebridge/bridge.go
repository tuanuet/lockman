package observebridge

import (
	"context"

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

// New constructs a Bridge from the given Config. Both Store and Dispatcher must
// be non-nil.
func New(cfg Config) *Bridge {
	if cfg.Store == nil {
		panic("observebridge: store must not be nil")
	}
	if cfg.Dispatcher == nil {
		panic("observebridge: dispatcher must not be nil")
	}
	return &Bridge{
		store:      cfg.Store,
		dispatcher: cfg.Dispatcher,
	}
}

// Shutdown delegates to the dispatcher's Shutdown, respecting the caller's
// context deadline without extending it.
func (b *Bridge) Shutdown(ctx context.Context) error {
	return b.dispatcher.Shutdown(ctx)
}

// publish applies the event to the local store first, then publishes one async
// copy to the dispatcher.
func (b *Bridge) publish(event observe.Event) {
	_ = b.store.Consume(context.Background(), event)
	b.dispatcher.Publish(event)
}
