package lockman

import (
	"context"
	"errors"
	"fmt"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/internal/sdk"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
)

// Hold acquires a detached lease and returns an opaque hold token.
func (c *Client) Hold(ctx context.Context, req HoldRequest) (HoldHandle, error) {
	if c == nil {
		return HoldHandle{}, fmt.Errorf("lockman: client is nil")
	}
	if c.shuttingDown.Load() {
		return HoldHandle{}, ErrShuttingDown
	}

	identity, err := c.validateHoldRequest(ctx, req)
	if err != nil {
		return HoldHandle{}, err
	}
	if c.holds == nil {
		return HoldHandle{}, ErrUseCaseNotFound
	}

	definitionID := normalizeUseCase(req.useCaseCore, map[string]int{}, req.registryLink).DefinitionID()
	token, err := sdk.EncodeHoldToken([]string{req.resourceKey}, identity.OwnerID)
	if err != nil {
		return HoldHandle{}, fmt.Errorf("lockman: encode hold token: %w", ErrHoldTokenInvalid)
	}

	_, err = c.holds.Acquire(ctx, definitions.DetachedAcquireRequest{
		DefinitionID: definitionID,
		ResourceKeys: []string{req.resourceKey},
		OwnerID:      identity.OwnerID,
	})
	if err != nil {
		return HoldHandle{}, mapHoldAcquireError(err, c.shuttingDown.Load())
	}

	return HoldHandle{token: token}, nil
}

// Forfeit releases a detached lease from a hold token.
func (c *Client) Forfeit(ctx context.Context, req ForfeitRequest) error {
	if c == nil {
		return fmt.Errorf("lockman: client is nil")
	}
	if c.shuttingDown.Load() {
		return ErrShuttingDown
	}
	if err := c.validateForfeitRequest(req); err != nil {
		return err
	}
	if c.holds == nil {
		return ErrUseCaseNotFound
	}

	resourceKeys, ownerID, err := sdk.DecodeHoldToken(req.token)
	if err != nil {
		return ErrHoldTokenInvalid
	}

	definitionID := normalizeUseCase(req.useCaseCore, map[string]int{}, req.registryLink).DefinitionID()
	err = c.holds.Release(ctx, definitions.DetachedReleaseRequest{
		DefinitionID: definitionID,
		ResourceKeys: resourceKeys,
		OwnerID:      ownerID,
	})

	return mapHoldReleaseError(err)
}

func mapHoldAcquireError(err error, shuttingDown bool) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, backend.ErrLeaseAlreadyHeld), errors.Is(err, backend.ErrLeaseOwnerMismatch):
		return ErrBusy
	case errors.Is(err, backend.ErrLeaseNotFound), errors.Is(err, backend.ErrLeaseExpired):
		return ErrHoldExpired
	case shuttingDown && errors.Is(err, lockerrors.ErrPolicyViolation):
		return ErrShuttingDown
	default:
		return err
	}
}

func mapHoldReleaseError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, backend.ErrLeaseNotFound), errors.Is(err, backend.ErrLeaseExpired):
		return ErrHoldExpired
	case errors.Is(err, backend.ErrLeaseAlreadyHeld), errors.Is(err, backend.ErrLeaseOwnerMismatch):
		return ErrBusy
	default:
		return err
	}
}
