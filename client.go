package lockman

import (
	"context"
	"sync/atomic"

	"lockman/backend"
	"lockman/lockkit/idempotency"
	lockruntime "lockman/lockkit/runtime"
	"lockman/lockkit/workers"
)

// Client executes registered run and claim use cases against the configured backend.
type Client struct {
	registry         *Registry
	backend          backend.Driver
	idempotency      idempotency.Store
	identity         Identity
	identityProvider func(context.Context) Identity
	runtime          *lockruntime.Manager
	worker           *workers.Manager
	shuttingDown     atomic.Bool
}

// New validates startup wiring and constructs the public SDK client.
func New(opts ...ClientOption) (*Client, error) {
	cfg := &clientConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	plan, err := buildClientPlan(cfg)
	if err != nil {
		return nil, err
	}

	client := &Client{
		registry:         cfg.registry,
		backend:          cfg.backend,
		idempotency:      cfg.idempotency,
		identity:         cfg.identity,
		identityProvider: cfg.identityProvider,
	}

	if plan.hasRunUseCases {
		client.runtime, err = lockruntime.NewManager(plan.engineRegistry, cfg.backend, nil)
		if err != nil {
			return nil, wrapStartupManagerError("runtime", err)
		}
	}
	if plan.hasClaimUseCases {
		client.worker, err = workers.NewManager(plan.engineRegistry, cfg.backend, cfg.idempotency)
		if err != nil {
			return nil, wrapStartupManagerError("worker", err)
		}
	}

	return client, nil
}

// Shutdown forwards shutdown to the underlying runtime and worker managers.
func (c *Client) Shutdown(ctx context.Context) error {
	if c == nil {
		return nil
	}

	c.shuttingDown.Store(true)

	var err error
	if c.runtime != nil {
		err = c.runtime.Shutdown(ctx)
	}
	if c.worker != nil {
		if shutdownErr := c.worker.Shutdown(ctx); shutdownErr != nil {
			if err == nil {
				err = shutdownErr
			} else {
				err = joinErrors(err, shutdownErr)
			}
		}
	}

	return err
}
