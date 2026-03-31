package runtime

import (
	"context"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
)

// CheckPresence reports advisory lock state for a registered, check-enabled definition.
func (m *Manager) CheckPresence(
	ctx context.Context,
	req definitions.PresenceCheckRequest,
) (definitions.PresenceStatus, error) {
	start := time.Now()

	def, err := m.getDefinition(req.DefinitionID)
	if err != nil {
		return definitions.PresenceStatus{State: definitions.PresenceUnknown}, err
	}
	defer func() {
		m.recordPresenceCheck(ctx, def.ID, start)
		if m.bridge != nil {
			m.bridge.PublishRuntimePresenceChecked(RuntimeEvent{
				DefinitionID: def.ID,
				OwnerID:      req.Ownership.OwnerID,
				RequestID:    req.Ownership.RequestID,
			})
		}
	}()

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

	record, err := m.driver.CheckPresence(ctx, backend.PresenceRequest{
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
