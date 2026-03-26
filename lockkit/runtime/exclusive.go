package runtime

import (
	"context"
	stdErrors "errors"
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

	key := lockKey(def.ID, resourceKey)
	if _, exists := m.active.Load(key); exists {
		return lockerrors.ErrReentrantAcquire
	}

	waitTimeout := resolveWaitTimeout(def.WaitTimeout, req.Overrides)
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
		return err
	}

	m.active.Store(key, struct{}{})
	m.recordActiveLocks(ctx, def.ID)

	leaseCtx := definitions.LeaseContext{
		DefinitionID:  def.ID,
		ResourceKey:   resourceKey,
		Ownership:     req.Ownership,
		LeaseTTL:      lease.LeaseTTL,
		LeaseDeadline: lease.ExpiresAt,
	}

	defer func() {
		m.active.Delete(key)
		m.recordActiveLocks(ctx, def.ID)
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

func resolveWaitTimeout(defWait time.Duration, overrides *definitions.RuntimeOverrides) time.Duration {
	if overrides == nil || overrides.WaitTimeout == nil {
		return defWait
	}
	override := *overrides.WaitTimeout
	if override < 0 {
		return 0
	}
	if defWait > 0 {
		if override > 0 && override < defWait {
			return override
		}
		return defWait
	}
	return override
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

func lockKey(definitionID, resourceKey string) string {
	return definitionID + lockKeySeparator + resourceKey
}
