package runtime

import (
	"context"
	"time"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	lockerrors "lockman/lockkit/errors"
)

// CheckPresence reports advisory lock state for a registered, check-enabled definition.
func (m *Manager) CheckPresence(
	ctx context.Context,
	req definitions.PresenceCheckRequest,
) (definitions.PresenceStatus, error) {
	start := time.Now()
	defer m.recordPresenceCheck(ctx, req.DefinitionID, start)

	def, err := m.getDefinition(req.DefinitionID)
	if err != nil {
		return definitions.PresenceStatus{State: definitions.PresenceUnknown}, err
	}

	status := definitions.PresenceStatus{
		State: definitions.PresenceUnknown,
		Mode:  def.Mode,
	}

	if !def.CheckOnlyAllowed {
		return status, lockerrors.ErrPolicyViolation
	}

	resourceKey, err := def.KeyBuilder.Build(req.KeyInput)
	if err != nil {
		return status, err
	}

	if err := m.driver.Ping(ctx); err != nil {
		return status, err
	}

	record, err := m.driver.CheckPresence(ctx, drivers.PresenceRequest{
		DefinitionID: def.ID,
		ResourceKeys: []string{resourceKey},
	})
	if err != nil {
		return status, err
	}

	if !record.Present {
		status.State = definitions.PresenceNotHeld
		return status, nil
	}

	status.State = definitions.PresenceHeld
	status.OwnerID = record.Lease.OwnerID
	status.LeaseDeadline = record.Lease.ExpiresAt
	return status, nil
}
