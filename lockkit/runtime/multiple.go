package runtime

import (
	"context"
	stdErrors "errors"
	"sort"
	"time"

	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/observe"
)

type acquiredMultipleLease struct {
	resourceKey string
	held        heldLease
}

// ExecuteMultipleExclusive runs fn after acquiring multiple keys of the same definition in canonical order.
// All keys must be acquired successfully (all-or-nothing). If any key fails, all previously acquired keys are released.
func (m *Manager) ExecuteMultipleExclusive(
	ctx context.Context,
	req definitions.MultipleLockRequest,
	fn func(context.Context, definitions.LeaseContext) error,
) (retErr error) {
	if m.isShuttingDown() {
		return lockerrors.ErrPolicyViolation
	}

	def, ok := m.getDefinition(req.DefinitionID)
	if !ok {
		return lockerrors.ErrPolicyViolation
	}
	if def.Mode == definitions.ModeStrict {
		return lockerrors.ErrPolicyViolation
	}
	if len(req.Keys) == 0 {
		return lockerrors.ErrPolicyViolation
	}
	if hasDuplicateKeys(req.Keys) {
		return lockerrors.ErrPolicyViolation
	}

	keys := make([]string, len(req.Keys))
	copy(keys, req.Keys)
	sort.Strings(keys)

	if !m.tryAdmitInFlightExecution() {
		return lockerrors.ErrPolicyViolation
	}
	admitted := true
	defer func() {
		if admitted {
			m.releaseInFlightExecution()
		}
	}()

	guardKeys := make([]guardKey, len(keys))
	for i, key := range keys {
		guardKeys[i] = guardKey{
			definitionID: def.ID,
			resourceKey:  key,
			ownerID:      req.Ownership.OwnerID,
		}
		if _, loaded := m.active.LoadOrStore(guardKeys[i], guardEntry{state: guardPending}); loaded {
			return lockerrors.ErrReentrantAcquire
		}
	}
	guardInstalled := true
	defer func() {
		if !guardInstalled {
			return
		}
		for _, key := range guardKeys {
			if v, ok := m.active.Load(key); ok {
				if entry, entryOk := v.(guardEntry); entryOk && entry.state == guardHeld {
					m.activeCounter(key.definitionID).Add(-1)
				}
			}
			m.active.Delete(key)
			m.recordActiveLocks(ctx, key.definitionID)
		}
	}()

	waitConfig, err := applyRuntimeOverrides(def, req.Overrides)
	if err != nil {
		return err
	}

	acquired := make([]acquiredMultipleLease, 0, len(keys))
	defer func() {
		for i := len(acquired) - 1; i >= 0; i-- {
			lease := acquired[i]
			held := time.Since(lease.held.lease.AcquiredAt)
			m.recorder.RecordRelease(ctx, def.ID, held)
			if m.bridge != nil {
				m.bridge.PublishRuntimeReleased(observe.Event{
					Kind:         observe.EventReleased,
					DefinitionID: def.ID,
					ResourceID:   lease.resourceKey,
					OwnerID:      req.Ownership.OwnerID,
					RequestID:    req.Ownership.RequestID,
					Held:         held,
				})
			}
			if releaseErr := m.releaseLease(context.Background(), lease.held); releaseErr != nil {
				if retErr == nil {
					retErr = releaseErr
				} else {
					retErr = stdErrors.Join(retErr, releaseErr)
				}
			}
		}
	}()

	for i, key := range keys {
		acquireCtx, cancel := contextWithAcquireTimeout(ctx, waitConfig)
		re := observe.Event{
			Kind:         observe.EventAcquireStarted,
			DefinitionID: def.ID,
			ResourceID:   key,
			OwnerID:      req.Ownership.OwnerID,
			RequestID:    req.Ownership.RequestID,
		}
		if m.bridge != nil {
			m.bridge.PublishRuntimeAcquireStarted(re)
		}
		start := time.Now()
		lease, acquireErr := m.acquireLease(acquireCtx, def, runtimeAcquirePlan{resourceKey: key}, req.Ownership.OwnerID)
		waitDuration := time.Since(start)
		cancel()

		re.Wait = waitDuration
		m.recorder.RecordAcquire(ctx, def.ID, waitDuration, acquireErr == nil)
		if acquireErr != nil {
			recordAcquireFailure(m, ctx, def.ID, acquireErr)
			if m.bridge != nil {
				m.bridge.PublishRuntimeAcquireFailed(re, acquireErr)
				recordBridgeAcquireFailure(m, re, acquireErr)
			}
			return mapAcquireError(acquireErr)
		}

		if m.bridge != nil {
			m.bridge.PublishRuntimeAcquireSucceeded(re)
		}

		acquired = append(acquired, acquiredMultipleLease{
			resourceKey: key,
			held:        lease,
		})
		m.active.Store(guardKeys[i], guardEntry{state: guardHeld})
		m.activeCounter(def.ID).Add(1)
		m.recordActiveLocks(ctx, def.ID)
	}

	retErr = fn(ctx, buildMultipleLeaseContext(req, acquired))
	return retErr
}

func hasDuplicateKeys(keys []string) bool {
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			return true
		}
		seen[key] = struct{}{}
	}
	return false
}

func buildMultipleLeaseContext(req definitions.MultipleLockRequest, acquired []acquiredMultipleLease) definitions.LeaseContext {
	resourceKeys := make([]string, len(acquired))
	var minTTL time.Duration
	var leaseDeadline time.Time

	for i, lease := range acquired {
		resourceKeys[i] = lease.resourceKey
		if i == 0 || lease.held.lease.LeaseTTL < minTTL {
			minTTL = lease.held.lease.LeaseTTL
		}
		if i == 0 || lease.held.lease.ExpiresAt.Before(leaseDeadline) {
			leaseDeadline = lease.held.lease.ExpiresAt
		}
	}

	return definitions.LeaseContext{
		DefinitionID:  req.DefinitionID,
		ResourceKeys:  resourceKeys,
		Ownership:     req.Ownership,
		LeaseTTL:      minTTL,
		LeaseDeadline: leaseDeadline,
	}
}
