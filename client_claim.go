package lockman

import (
	"context"
	"fmt"

	"github.com/tuanuet/lockman/internal/sdk"
	"github.com/tuanuet/lockman/lockkit/definitions"
)

// Claim executes an asynchronous claim use case through the existing worker manager.
func (c *Client) Claim(ctx context.Context, req ClaimRequest, fn func(context.Context, Claim) error) error {
	if c == nil {
		return fmt.Errorf("lockman: client is nil")
	}
	if fn == nil {
		return fmt.Errorf("lockman: claim callback is required")
	}
	if c.shuttingDown.Load() {
		return ErrShuttingDown
	}

	normalized, identity, err := c.validateClaimRequest(ctx, req)
	if err != nil {
		return err
	}
	if c.worker == nil {
		return ErrUseCaseNotFound
	}

	translated := sdk.TranslateClaim(normalized)
	translated.Ownership.ServiceName = identity.Service
	translated.Ownership.InstanceID = identity.Instance

	err = c.worker.ExecuteClaimed(ctx, translated, func(ctx context.Context, claim definitions.ClaimContext) error {
		return fn(ctx, Claim{
			UseCase:        req.useCaseName,
			ResourceKey:    claim.ResourceKey,
			LeaseTTL:       claim.LeaseTTL,
			Deadline:       claim.LeaseDeadline,
			FencingToken:   claim.FencingToken,
			IdempotencyKey: claim.IdempotencyKey,
		})
	})

	return mapEngineError(err, c.shuttingDown.Load())
}
