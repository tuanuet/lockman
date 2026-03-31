package lockman

import (
	"context"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/idempotency"
	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/observe"
)

// Identity identifies the caller for lock ownership and diagnostics.
type Identity struct {
	OwnerID  string
	Service  string
	Instance string
}

// ClientOption configures a client.
type ClientOption func(*clientConfig)

// CallOption configures a single use-case invocation.
type CallOption func(*callConfig)

// Observability bundles the optional observability dependencies for the client.
type Observability struct {
	Dispatcher observe.Dispatcher
	Store      *inspect.Store
}

type clientConfig struct {
	identity         Identity
	identityProvider func(context.Context) Identity
	registry         *Registry
	backend          backend.Driver
	idempotency      idempotency.Store
	observer         observe.Dispatcher
	inspectStore     *inspect.Store
}

// WithIdentity sets a static caller identity for the client.
func WithIdentity(identity Identity) ClientOption {
	return func(cfg *clientConfig) {
		cfg.identity = identity
	}
}

// WithIdentityProvider sets a dynamic caller identity provider for the client.
func WithIdentityProvider(provider func(context.Context) Identity) ClientOption {
	return func(cfg *clientConfig) {
		cfg.identityProvider = provider
	}
}

// WithRegistry binds the client to the centralized use-case registry.
func WithRegistry(registry *Registry) ClientOption {
	return func(cfg *clientConfig) {
		cfg.registry = registry
	}
}

// WithBackend sets the lock backend used by runtime and worker flows.
func WithBackend(drv backend.Driver) ClientOption {
	return func(cfg *clientConfig) {
		cfg.backend = drv
	}
}

// WithIdempotency sets the idempotency store for claim-based flows.
func WithIdempotency(store idempotency.Store) ClientOption {
	return func(cfg *clientConfig) {
		cfg.idempotency = store
	}
}

// WithObserver sets the observe dispatcher for the client.
func WithObserver(dispatcher observe.Dispatcher) ClientOption {
	return func(cfg *clientConfig) {
		cfg.observer = dispatcher
	}
}

// WithInspectStore sets the inspect store for the client.
func WithInspectStore(store *inspect.Store) ClientOption {
	return func(cfg *clientConfig) {
		cfg.inspectStore = store
	}
}

// WithObservability sets both the observe dispatcher and inspect store from a
// single Observability bundle.
func WithObservability(obs Observability) ClientOption {
	return func(cfg *clientConfig) {
		cfg.observer = obs.Dispatcher
		cfg.inspectStore = obs.Store
	}
}
