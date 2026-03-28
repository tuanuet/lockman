package lockman

import (
	"context"
	"fmt"

	"lockman/internal/sdk"
	"lockman/lockkit/definitions"
)

// Run executes a synchronous use case through the existing runtime manager.
func (c *Client) Run(ctx context.Context, req RunRequest, fn func(context.Context, Lease) error) error {
	if c == nil {
		return fmt.Errorf("lockman: client is nil")
	}
	if fn == nil {
		return fmt.Errorf("lockman: run callback is required")
	}
	if c.shuttingDown.Load() {
		return ErrShuttingDown
	}

	normalized, identity, err := c.validateRunRequest(ctx, req)
	if err != nil {
		return err
	}
	if c.runtime == nil {
		return ErrUseCaseNotFound
	}

	translated := sdk.TranslateRun(normalized)
	translated.Ownership.ServiceName = identity.Service
	translated.Ownership.InstanceID = identity.Instance

	err = c.runtime.ExecuteExclusive(ctx, translated, func(ctx context.Context, lease definitions.LeaseContext) error {
		return fn(ctx, Lease{
			UseCase:      req.useCaseName,
			ResourceKey:  lease.ResourceKey,
			LeaseTTL:     lease.LeaseTTL,
			Deadline:     lease.LeaseDeadline,
			FencingToken: lease.FencingToken,
		})
	})

	return mapEngineError(err, c.shuttingDown.Load())
}
