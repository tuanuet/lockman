package lockman

import "context"

// Identity identifies the caller for lock ownership and diagnostics.
type Identity struct {
	OwnerID  string
	Service  string
	Instance string
}

// Client is a placeholder root SDK client entry point for now.
type Client struct{}

// ClientOption configures a client.
type ClientOption func(*clientConfig)

// CallOption configures a single use-case invocation.
type CallOption func(*callConfig)

type clientConfig struct {
	identity         Identity
	identityProvider func(context.Context) Identity
}

type callConfig struct {
	ownerID string
}

// New is a placeholder constructor for the root SDK surface.
// It currently always returns ErrRegistryRequired until registry/backend
// wiring is implemented in later tasks. Options are accepted now only to
// pin the intended public SDK API shape.
func New(opts ...ClientOption) (*Client, error) {
	cfg := &clientConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	return nil, ErrRegistryRequired
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

// OwnerID overrides the owner identity for one call.
func OwnerID(id string) CallOption {
	return func(cfg *callConfig) {
		cfg.ownerID = id
	}
}
