package lockman

import (
	"context"
	"fmt"

	"github.com/tuanuet/lockman/internal/sdk"
	"github.com/tuanuet/lockman/lockkit/definitions"
)

const maxMultipleKeys = 100

// RunMultiple acquires multiple keys of the same definition atomically (all-or-nothing)
// and executes the callback after all keys are acquired.
//
// All requests must belong to the same use case and be built via RunUseCase.With.
func (c *Client) RunMultiple(
	ctx context.Context,
	fn func(ctx context.Context, lease Lease) error,
	requests []RunRequest,
) error {
	if c == nil {
		return fmt.Errorf("lockman: client is nil")
	}
	if fn == nil {
		return fmt.Errorf("lockman: run multiple callback is required")
	}
	if c.shuttingDown.Load() {
		return ErrShuttingDown
	}

	keys, uc, identity, err := c.extractRunRequests(ctx, requests)
	if err != nil {
		return err
	}

	if c.runtime == nil {
		return ErrUseCaseNotFound
	}

	definitionID := c.plan.definitionIDByUseCase[uc.name]
	if definitionID == "" {
		return ErrUseCaseNotFound
	}

	multipleReq := definitions.MultipleLockRequest{
		DefinitionID: definitionID,
		Keys:         keys,
		Ownership: definitions.OwnershipMeta{
			ServiceName: identity.Service,
			InstanceID:  identity.Instance,
			HandlerName: uc.name,
			OwnerID:     identity.OwnerID,
		},
	}

	err = c.runtime.ExecuteMultipleExclusive(ctx, multipleReq, func(ctx context.Context, lease definitions.LeaseContext) error {
		return fn(ctx, Lease{
			UseCase:      uc.name,
			ResourceKeys: append([]string(nil), lease.ResourceKeys...),
			LeaseTTL:     lease.LeaseTTL,
			Deadline:     lease.LeaseDeadline,
			FencingToken: lease.FencingToken,
		})
	})

	return mapEngineError(err, c.shuttingDown.Load())
}

// HoldMultiple acquires multiple keys of the same definition atomically and returns
// a single HoldHandle that manages all acquired keys.
//
// All requests must belong to the same use case and be built via HoldUseCase.With.
//
// Note: HoldMultiple uses the hold manager's direct acquire path (same as single-key Hold),
// not the runtime engine. The hold manager's Acquire already supports ResourceKeys (plural)
// via DetachedAcquireRequest. This means HoldMultiple does not get reentrancy guards or
// canonical ordering from the engine — it relies on the hold manager's backend-level
// atomicity. For strong ordering/guard guarantees, use RunMultiple instead.
func (c *Client) HoldMultiple(
	ctx context.Context,
	requests []HoldRequest,
) (HoldHandle, error) {
	if c == nil {
		return HoldHandle{}, fmt.Errorf("lockman: client is nil")
	}
	if c.shuttingDown.Load() {
		return HoldHandle{}, ErrShuttingDown
	}

	keys, uc, identity, err := c.extractHoldRequests(ctx, requests)
	if err != nil {
		return HoldHandle{}, err
	}

	if c.holds == nil {
		return HoldHandle{}, ErrUseCaseNotFound
	}

	definitionID := c.plan.definitionIDByUseCase[uc.name]
	if definitionID == "" {
		return HoldHandle{}, ErrUseCaseNotFound
	}

	token, err := sdk.EncodeHoldToken(keys, identity.OwnerID)
	if err != nil {
		return HoldHandle{}, fmt.Errorf("lockman: encode hold token: %w", ErrHoldTokenInvalid)
	}

	_, err = c.holds.Acquire(ctx, definitions.DetachedAcquireRequest{
		DefinitionID: definitionID,
		ResourceKeys: keys,
		OwnerID:      identity.OwnerID,
	})
	if err != nil {
		return HoldHandle{}, mapHoldAcquireError(err, c.shuttingDown.Load())
	}

	return HoldHandle{token: token}, nil
}

func (c *Client) extractRunRequests(ctx context.Context, requests []RunRequest) ([]string, *useCaseCore, Identity, error) {
	if len(requests) == 0 {
		return nil, nil, Identity{}, fmt.Errorf("lockman: requests must not be empty")
	}
	if len(requests) > maxMultipleKeys {
		return nil, nil, Identity{}, fmt.Errorf("lockman: requests must not exceed %d", maxMultipleKeys)
	}

	keys := make([]string, len(requests))
	seen := make(map[string]struct{}, len(requests))
	var uc *useCaseCore
	for i, req := range requests {
		if req.useCaseCore == nil {
			return nil, nil, Identity{}, fmt.Errorf("lockman: request %d has no use case", i)
		}
		if uc == nil {
			uc = req.useCaseCore
		} else if uc != req.useCaseCore {
			return nil, nil, Identity{}, fmt.Errorf("lockman: all requests must belong to the same use case")
		}
		key := req.resourceKey
		if _, ok := seen[key]; ok {
			return nil, nil, Identity{}, fmt.Errorf("lockman: duplicate key %q", key)
		}
		seen[key] = struct{}{}
		keys[i] = key
	}

	identity, err := c.validateRegisteredUseCase(ctx, uc)
	if err != nil {
		return nil, nil, Identity{}, err
	}

	return keys, uc, identity, nil
}

func (c *Client) extractHoldRequests(ctx context.Context, requests []HoldRequest) ([]string, *useCaseCore, Identity, error) {
	if len(requests) == 0 {
		return nil, nil, Identity{}, fmt.Errorf("lockman: requests must not be empty")
	}
	if len(requests) > maxMultipleKeys {
		return nil, nil, Identity{}, fmt.Errorf("lockman: requests must not exceed %d", maxMultipleKeys)
	}

	keys := make([]string, len(requests))
	seen := make(map[string]struct{}, len(requests))
	var uc *useCaseCore
	for i, req := range requests {
		if req.useCaseCore == nil {
			return nil, nil, Identity{}, fmt.Errorf("lockman: request %d has no use case", i)
		}
		if uc == nil {
			uc = req.useCaseCore
		} else if uc != req.useCaseCore {
			return nil, nil, Identity{}, fmt.Errorf("lockman: all requests must belong to the same use case")
		}
		key := req.resourceKey
		if _, ok := seen[key]; ok {
			return nil, nil, Identity{}, fmt.Errorf("lockman: duplicate key %q", key)
		}
		seen[key] = struct{}{}
		keys[i] = key
	}

	identity, err := c.validateRegisteredUseCase(ctx, uc)
	if err != nil {
		return nil, nil, Identity{}, err
	}

	return keys, uc, identity, nil
}

func (c *Client) validateRegisteredUseCase(ctx context.Context, uc *useCaseCore) (Identity, error) {
	if uc == nil {
		return Identity{}, ErrUseCaseNotFound
	}
	if uc.registry == nil {
		return Identity{}, fmt.Errorf("lockman: use case %q is not registered: %w", uc.name, ErrUseCaseNotFound)
	}
	if c.registry == nil || sdk.RegistryLinkMismatch(c.registry.link, uc.registry.link) {
		return Identity{}, fmt.Errorf("lockman: use case %q belongs to a different registry: %w", uc.name, ErrRegistryMismatch)
	}

	return c.resolveIdentity(ctx, "")
}
