package runtime

import (
	"context"
	stdErrors "errors"
	"time"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	lockerrors "lockman/lockkit/errors"
)

type guardKey struct {
	definitionID string
	resourceKey  string
	ownerID      string
}

type guardState int

const (
	guardPending guardState = iota
	guardHeld
)

type guardEntry struct {
	state guardState
}

// ExecuteExclusive runs fn after successfully acquiring the requested standard lock.
func (m *Manager) ExecuteExclusive(
	ctx context.Context,
	req definitions.SyncLockRequest,
	fn func(context.Context, definitions.LeaseContext) error,
) (retErr error) {
	def, err := m.getDefinition(req.DefinitionID)
	if err != nil {
		return err
	}

	resourceKey, err := def.KeyBuilder.Build(req.KeyInput)
	if err != nil {
		return err
	}

	waitConfig, err := applyRuntimeOverrides(def, req.Overrides)
	if err != nil {
		return err
	}

	key := guardKey{definitionID: def.ID, resourceKey: resourceKey, ownerID: req.Ownership.OwnerID}
	entry := guardEntry{state: guardPending}
	if _, loaded := m.active.LoadOrStore(key, entry); loaded {
		return lockerrors.ErrReentrantAcquire
	}

	acquireCtx, cancel := contextWithAcquireTimeout(ctx, waitConfig)
	defer cancel()

	var lease drivers.LeaseRecord
	var leaseAcquired bool
	defer func() {
		if leaseAcquired {
			held := time.Since(lease.AcquiredAt)
			m.recorder.RecordRelease(ctx, def.ID, held)
			if releaseErr := m.driver.Release(context.Background(), lease); releaseErr != nil {
				if retErr == nil {
					retErr = releaseErr
				} else {
					retErr = stdErrors.Join(retErr, releaseErr)
				}
			}
		}
		m.active.Delete(key)
		m.recordActiveLocks(ctx, def.ID)
	}()

	start := time.Now()
	lease, err = m.driver.Acquire(acquireCtx, drivers.AcquireRequest{
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

	leaseAcquired = true
	m.active.Store(key, guardEntry{state: guardHeld})
	m.recordActiveLocks(ctx, def.ID)
	leaseCtx := definitions.LeaseContext{
		DefinitionID:  def.ID,
		ResourceKey:   resourceKey,
		Ownership:     req.Ownership,
		LeaseTTL:      lease.LeaseTTL,
		LeaseDeadline: lease.ExpiresAt,
	}

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

type waitConfig struct {
	timeout  time.Duration
	explicit bool
}

func applyRuntimeOverrides(def definitions.LockDefinition, overrides *definitions.RuntimeOverrides) (waitConfig, error) {
	cfg := waitConfig{timeout: def.WaitTimeout}
	if overrides == nil {
		return cfg, nil
	}
	if overrides.MaxRetries != nil {
		return waitConfig{}, lockerrors.ErrPolicyViolation
	}
	if overrides.WaitTimeout == nil {
		return cfg, nil
	}

	wait := *overrides.WaitTimeout
	if wait < 0 {
		return waitConfig{}, lockerrors.ErrPolicyViolation
	}
	if def.WaitTimeout > 0 && wait > def.WaitTimeout {
		return waitConfig{}, lockerrors.ErrPolicyViolation
	}

	cfg.timeout = wait
	cfg.explicit = true
	return cfg, nil
}

func mapAcquireError(err error) error {
	switch {
	case stdErrors.Is(err, drivers.ErrLeaseAlreadyHeld):
		return lockerrors.ErrLockBusy
	case stdErrors.Is(err, context.DeadlineExceeded):
		return lockerrors.ErrLockAcquireTimeout
	default:
		return err
	}
}

func contextWithAcquireTimeout(ctx context.Context, cfg waitConfig) (context.Context, context.CancelFunc) {
	if cfg.timeout <= 0 {
		if cfg.explicit {
			return context.WithDeadline(ctx, time.Now())
		}
		return ctx, func() {}
	}

	deadline := time.Now().Add(cfg.timeout)
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
	m.active.Range(func(key, value interface{}) bool {
		gk, ok := key.(guardKey)
		if !ok || gk.definitionID != definitionID {
			return true
		}
		entry, ok := value.(guardEntry)
		if ok && entry.state == guardHeld {
			count++
		}
		return true
	})
	return count
}

func (m *Manager) getDefinition(id string) (def definitions.LockDefinition, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = lockerrors.ErrPolicyViolation
		}
	}()
	def = m.registry.MustGet(id)
	return def, err
}
