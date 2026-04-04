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
func (c *Client) RunMultiple(
	ctx context.Context,
	uc registeredUseCase,
	fn func(ctx context.Context, lease Lease) error,
	input any,
	keys []string,
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

	if err := validateMultipleKeys(keys); err != nil {
		return err
	}

	core := uc.sdkUseCase()
	if core == nil {
		return ErrUseCaseNotFound
	}

	identity, err := c.validateRegisteredUseCase(ctx, uc)
	if err != nil {
		return err
	}
	if c.runtime == nil {
		return ErrUseCaseNotFound
	}

	definitionID := c.plan.definitionIDByUseCase[core.name]
	if definitionID == "" {
		return ErrUseCaseNotFound
	}

	multipleReq := definitions.MultipleLockRequest{
		DefinitionID: definitionID,
		Keys:         keys,
		Ownership: definitions.OwnershipMeta{
			ServiceName: identity.Service,
			InstanceID:  identity.Instance,
			HandlerName: core.name,
			OwnerID:     identity.OwnerID,
		},
	}

	err = c.runtime.ExecuteMultipleExclusive(ctx, multipleReq, func(ctx context.Context, lease definitions.LeaseContext) error {
		return fn(ctx, Lease{
			UseCase:      core.name,
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
// Note: HoldMultiple uses the hold manager's direct acquire path (same as single-key Hold),
// not the runtime engine. The hold manager's Acquire already supports ResourceKeys (plural)
// via DetachedAcquireRequest. This means HoldMultiple does not get reentrancy guards or
// canonical ordering from the engine — it relies on the hold manager's backend-level
// atomicity. For strong ordering/guard guarantees, use RunMultiple instead.
func (c *Client) HoldMultiple(
	ctx context.Context,
	uc registeredUseCase,
	input any,
	keys []string,
) (HoldHandle, error) {
	if c == nil {
		return HoldHandle{}, fmt.Errorf("lockman: client is nil")
	}
	if c.shuttingDown.Load() {
		return HoldHandle{}, ErrShuttingDown
	}

	if err := validateMultipleKeys(keys); err != nil {
		return HoldHandle{}, err
	}

	core := uc.sdkUseCase()
	if core == nil {
		return HoldHandle{}, ErrUseCaseNotFound
	}

	identity, err := c.validateRegisteredUseCase(ctx, uc)
	if err != nil {
		return HoldHandle{}, err
	}
	if c.holds == nil {
		return HoldHandle{}, ErrUseCaseNotFound
	}

	definitionID := c.plan.definitionIDByUseCase[core.name]
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

func validateMultipleKeys(keys []string) error {
	if len(keys) == 0 {
		return fmt.Errorf("lockman: keys must not be empty")
	}
	if len(keys) > maxMultipleKeys {
		return fmt.Errorf("lockman: keys must not exceed %d", maxMultipleKeys)
	}
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			return fmt.Errorf("lockman: duplicate key %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (c *Client) validateRegisteredUseCase(ctx context.Context, uc registeredUseCase) (Identity, error) {
	core := uc.sdkUseCase()
	if core == nil {
		return Identity{}, ErrUseCaseNotFound
	}
	if core.registry == nil {
		return Identity{}, fmt.Errorf("lockman: use case %q is not registered: %w", core.name, ErrUseCaseNotFound)
	}
	if c.registry == nil || sdk.RegistryLinkMismatch(c.registry.link, core.registry.link) {
		return Identity{}, fmt.Errorf("lockman: use case %q belongs to a different registry: %w", core.name, ErrRegistryMismatch)
	}

	return c.resolveIdentity(ctx, "")
}
