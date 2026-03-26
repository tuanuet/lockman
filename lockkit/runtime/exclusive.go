package runtime

import (
	"context"
	stdErrors "errors"
	"fmt"
	"strings"
	"time"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	lockerrors "lockman/lockkit/errors"
)

const lockKeySeparator = ":"

// ExecuteExclusive runs fn after successfully acquiring the requested standard lock.
func (m *Manager) ExecuteExclusive(
	ctx context.Context,
	req definitions.SyncLockRequest,
	fn func(context.Context, definitions.LeaseContext) error,
) (retErr error) {
	def := m.registry.MustGet(req.DefinitionID)

	resourceKey, err := def.KeyBuilder.Build(req.KeyInput)
	if err != nil {
		return err
	}

	waitTimeout, err := applyRuntimeOverrides(def, req.Overrides)
	if err != nil {
		return err
	}

	key := lockKey(def.ID, resourceKey, req.Ownership.OwnerID)
	if _, loaded := m.active.LoadOrStore(key, struct{}{}); loaded {
		return lockerrors.ErrReentrantAcquire
	}
	m.recordActiveLocks(ctx, def.ID)
	guardActive := true
	defer func() {
		if guardActive {
			m.active.Delete(key)
			m.recordActiveLocks(ctx, def.ID)
		}
	}()

	acquireCtx, cancel := contextWithAcquireTimeout(ctx, waitTimeout)
	defer cancel()

	start := time.Now()
	lease, err := m.driver.Acquire(acquireCtx, drivers.AcquireRequest{
		DefinitionID: def.ID,
		ResourceKeys: []string{resourceKey},
		OwnerID:      req.Ownership.OwnerID,
		LeaseTTL:     def.LeaseTTL,
	})
	waitDuration := time.Since(start)
	m.recorder.RecordAcquire(ctx, def.ID, waitDuration, err == nil)

	if err != nil {
		recordAcquireFailure(m, ctx, def.ID, err)
		return mapAcquireError(err)
	}

	leaseCtx := definitions.LeaseContext{
		DefinitionID:  def.ID,
		ResourceKey:   resourceKey,
		Ownership:     req.Ownership,
		LeaseTTL:      lease.LeaseTTL,
		LeaseDeadline: lease.ExpiresAt,
	}

	defer func() {
		held := time.Since(lease.AcquiredAt)
		m.recorder.RecordRelease(ctx, def.ID, held)
		if releaseErr := m.driver.Release(context.Background(), lease); releaseErr != nil {
			if retErr == nil {
				retErr = releaseErr
			} else {
				retErr = stdErrors.Join(retErr, releaseErr)
			}
		}
	}()

	retErr = fn(ctx, leaseCtx)
	return retErr
}

func recordAcquireFailure(m *Manager, ctx context.Context, definitionID string, err error) {
	if stdErrors.Is(err, context.DeadlineExceeded) {
		m.recorder.RecordTimeout(ctx, definitionID)
	}
	if stdErrors.Is(err, drivers.ErrLeaseAlreadyHeld) {
		m.recorder.RecordContention(ctx, definitionID)
	}
}

func applyRuntimeOverrides(def definitions.LockDefinition, overrides *definitions.RuntimeOverrides) (time.Duration, error) {
	if overrides == nil {
		return def.WaitTimeout, nil
	}
	if overrides.MaxRetries != nil {
		return 0, lockerrors.ErrPolicyViolation
	}
	if overrides.WaitTimeout == nil {
		return def.WaitTimeout, nil
	}

	wait := *overrides.WaitTimeout
	if wait < 0 {
		return 0, lockerrors.ErrPolicyViolation
	}
	if def.WaitTimeout > 0 && wait > def.WaitTimeout {
		return 0, lockerrors.ErrPolicyViolation
	}
	return wait, nil
}

func mapAcquireError(err error) error {
	switch {
	case stdErrors.Is(err, drivers.ErrLeaseAlreadyHeld):
		return lockerrors.ErrLockBusy
	case stdErrors.Is(err, context.DeadlineExceeded), stdErrors.Is(err, context.Canceled):
		return lockerrors.ErrLockAcquireTimeout
	default:
		return err
	}
}

func contextWithAcquireTimeout(ctx context.Context, waitTimeout time.Duration) (context.Context, context.CancelFunc) {
	if waitTimeout <= 0 {
		return ctx, func() {}
	}

	deadline := time.Now().Add(waitTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok {
		if !deadline.Before(ctxDeadline) {
			return ctx, func() {}
		}
	}
	return context.WithDeadline(ctx, deadline)
}

func (m *Manager) recordActiveLocks(ctx context.Context, definitionID string) {
	count := m.activeCount(definitionID)
	m.recorder.RecordActiveLocks(ctx, definitionID, count)
}

func (m *Manager) activeCount(definitionID string) int {
	count := 0
	prefix := definitionID + lockKeySeparator
	m.active.Range(func(key, _ interface{}) bool {
		str, ok := key.(string)
		if !ok {
			return true
		}
		if strings.HasPrefix(str, prefix) {
			count++
		}
		return true
	})
	return count
}

func lockKey(definitionID, resourceKey, ownerID string) string {
	return fmt.Sprintf("%s%s%s%s%s", definitionID, lockKeySeparator, resourceKey, lockKeySeparator, ownerID)
}
