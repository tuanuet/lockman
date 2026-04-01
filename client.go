package lockman

import (
	"context"
	"sync/atomic"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/idempotency"
	"github.com/tuanuet/lockman/internal/observebridge"
	lockruntime "github.com/tuanuet/lockman/lockkit/runtime"
	"github.com/tuanuet/lockman/lockkit/workers"
	"github.com/tuanuet/lockman/observe"
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
	bridge           *observebridge.Bridge
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

	var store observe.Sink
	if cfg.inspectStore != nil {
		store = cfg.inspectStore
	}
	if store != nil || cfg.observer != nil {
		client.bridge = observebridge.New(observebridge.Config{
			Store:      store,
			Dispatcher: cfg.observer,
		})
	}

	if plan.hasRunUseCases {
		var runtimeOpts []lockruntime.Option
		if client.bridge != nil {
			runtimeOpts = append(runtimeOpts, lockruntime.WithBridge(client.bridge))
		}
		client.runtime, err = lockruntime.NewManager(plan.engineRegistry, cfg.backend, nil, runtimeOpts...)
		if err != nil {
			return nil, wrapStartupManagerError("runtime", err)
		}
	}
	if plan.hasClaimUseCases {
		var workerOpts []workers.Option
		if client.bridge != nil {
			workerOpts = append(workerOpts, workers.WithBridge(client.bridge))
		}
		client.worker, err = workers.NewManager(plan.engineRegistry, cfg.backend, cfg.idempotency, workerOpts...)
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

	if c.bridge != nil {
		c.bridge.PublishClientShutdownStarted()
	}

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

	if c.bridge != nil {
		c.bridge.PublishClientShutdownCompleted()
		if shutdownErr := c.bridge.Shutdown(ctx); shutdownErr != nil {
			if err == nil {
				err = shutdownErr
			} else {
				err = joinErrors(err, shutdownErr)
			}
		}
	}

	return err
}
