package lockman

import (
	"context"

	"lockman/lockkit/drivers"
	"lockman/lockkit/idempotency"
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

type clientConfig struct {
	identity         Identity
	identityProvider func(context.Context) Identity
	registry         *Registry
	backend          drivers.Driver
	idempotency      idempotency.Store
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
func WithBackend(backend drivers.Driver) ClientOption {
	return func(cfg *clientConfig) {
		cfg.backend = backend
	}
}

// WithIdempotency sets the idempotency store for claim-based flows.
func WithIdempotency(store idempotency.Store) ClientOption {
	return func(cfg *clientConfig) {
		cfg.idempotency = store
	}
}
