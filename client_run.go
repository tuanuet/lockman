package lockman

import (
	"context"
	"fmt"

	"github.com/tuanuet/lockman/internal/sdk"
	"github.com/tuanuet/lockman/lockkit/definitions"
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

	if len(req.compositeMemberInputs) > 0 {
		compositeDefinitionID := normalizeUseCase(req.useCaseCore, map[string]int{}, req.registryLink).DefinitionID()
		compositeRequest := definitions.CompositeLockRequest{
			DefinitionID: compositeDefinitionID,
			MemberInputs: req.compositeMemberInputs,
			Ownership: definitions.OwnershipMeta{
				ServiceName: identity.Service,
				InstanceID:  identity.Instance,
				HandlerName: req.useCaseName,
				OwnerID:     identity.OwnerID,
			},
		}

		err = c.runtime.ExecuteCompositeExclusive(ctx, compositeRequest, func(ctx context.Context, lease definitions.LeaseContext) error {
			return fn(ctx, Lease{
				UseCase:      req.useCaseName,
				ResourceKey:  lease.ResourceKey,
				ResourceKeys: append([]string(nil), lease.ResourceKeys...),
				LeaseTTL:     lease.LeaseTTL,
				Deadline:     lease.LeaseDeadline,
				FencingToken: lease.FencingToken,
			})
		})

		return mapEngineError(err, c.shuttingDown.Load())
	}

	translated := sdk.TranslateRun(normalized)
	if c.plan.lineageDefinitionIDs != nil && !c.plan.lineageDefinitionIDs[translated.DefinitionID] {
		translated.ResourceKey = translated.KeyInput[sdk.ResourceKeyInputKey]
		translated.KeyInput = nil
	}
	translated.Ownership.ServiceName = identity.Service
	translated.Ownership.InstanceID = identity.Instance

	err = c.runtime.ExecuteExclusive(ctx, translated, func(ctx context.Context, lease definitions.LeaseContext) error {
		return fn(ctx, Lease{
			UseCase:      req.useCaseName,
			ResourceKey:  lease.ResourceKey,
			ResourceKeys: append([]string(nil), lease.ResourceKeys...),
			LeaseTTL:     lease.LeaseTTL,
			Deadline:     lease.LeaseDeadline,
			FencingToken: lease.FencingToken,
		})
	})

	return mapEngineError(err, c.shuttingDown.Load())
}
