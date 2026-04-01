package runtime

import (
	"context"
	stdErrors "errors"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/internal/lineage"
	"github.com/tuanuet/lockman/observe"
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

type runtimeAcquirePlan struct {
	resourceKey string
	lineage     *backend.LineageLeaseMeta
}

type heldLease struct {
	lease        backend.LeaseRecord
	lineage      *backend.LineageLeaseMeta
	fencingToken uint64
}

// ExecuteExclusive runs fn after successfully acquiring the requested standard lock.
func (m *Manager) ExecuteExclusive(
	ctx context.Context,
	req definitions.SyncLockRequest,
	fn func(context.Context, definitions.LeaseContext) error,
) (retErr error) {
	if m.isShuttingDown() {
		return lockerrors.ErrPolicyViolation
	}

	def, err := m.getDefinition(req.DefinitionID)
	if err != nil {
		return err
	}

	acquirePlan, err := m.buildAcquirePlan(def, req.KeyInput)
	if err != nil {
		return err
	}
	resourceKey := acquirePlan.resourceKey

	waitConfig, err := applyRuntimeOverrides(def, req.Overrides)
	if err != nil {
		return err
	}

	if !m.tryAdmitInFlightExecution() {
		return lockerrors.ErrPolicyViolation
	}
	admitted := true
	defer func() {
		if admitted {
			m.releaseInFlightExecution()
		}
	}()

	key := guardKey{definitionID: def.ID, resourceKey: resourceKey, ownerID: req.Ownership.OwnerID}
	entry := guardEntry{state: guardPending}
	if _, loaded := m.active.LoadOrStore(key, entry); loaded {
		return lockerrors.ErrReentrantAcquire
	}
	guardInstalled := true

	acquireCtx, cancel := contextWithAcquireTimeout(ctx, waitConfig)
	defer cancel()

	var lease heldLease
	var leaseAcquired bool
	defer func() {
		if leaseAcquired {
			held := time.Since(lease.lease.AcquiredAt)
			m.recorder.RecordRelease(ctx, def.ID, held)
			if m.bridge != nil {
				m.bridge.PublishRuntimeReleased(observe.Event{
					Kind:         observe.EventReleased,
					DefinitionID: def.ID,
					ResourceID:   resourceKey,
					OwnerID:      req.Ownership.OwnerID,
					RequestID:    req.Ownership.RequestID,
					Held:         held,
				})
			}
			if releaseErr := m.releaseLease(context.Background(), lease); releaseErr != nil {
				if retErr == nil {
					retErr = releaseErr
				} else {
					retErr = stdErrors.Join(retErr, releaseErr)
				}
			}
		}
		if guardInstalled {
			if v, ok := m.active.Load(key); ok {
				if entry, entryOk := v.(guardEntry); entryOk && entry.state == guardHeld {
					m.activeCounter(key.definitionID).Add(-1)
				}
			}
			m.active.Delete(key)
			m.recordActiveLocks(ctx, def.ID)
		}
	}()

	start := time.Now()
	re := observe.Event{
		Kind:         observe.EventAcquireStarted,
		DefinitionID: def.ID,
		ResourceID:   resourceKey,
		OwnerID:      req.Ownership.OwnerID,
		RequestID:    req.Ownership.RequestID,
	}
	if m.bridge != nil {
		m.bridge.PublishRuntimeAcquireStarted(re)
	}
	lease, err = m.acquireLease(acquireCtx, def, acquirePlan, req.Ownership.OwnerID)
	waitDuration := time.Since(start)
	re.Wait = waitDuration
	m.recorder.RecordAcquire(ctx, def.ID, waitDuration, err == nil)

	if err != nil {
		recordAcquireFailure(m, ctx, def.ID, err)
		if m.bridge != nil {
			m.bridge.PublishRuntimeAcquireFailed(re, err)
			recordBridgeAcquireFailure(m, re, err)
		}
		return mapAcquireError(err)
	}

	if m.bridge != nil {
		m.bridge.PublishRuntimeAcquireSucceeded(re)
	}

	leaseAcquired = true

	m.active.Store(key, guardEntry{state: guardHeld})
	m.activeCounter(def.ID).Add(1)
	m.recordActiveLocks(ctx, def.ID)
	leaseCtx := definitions.LeaseContext{
		DefinitionID:  def.ID,
		ResourceKey:   resourceKey,
		Ownership:     req.Ownership,
		LeaseTTL:      lease.lease.LeaseTTL,
		LeaseDeadline: lease.lease.ExpiresAt,
		FencingToken:  lease.fencingToken,
	}

	retErr = fn(ctx, leaseCtx)
	return retErr
}

func recordAcquireFailure(m *Manager, ctx context.Context, definitionID string, err error) {
	if stdErrors.Is(err, lockerrors.ErrOverlapRejected) {
		m.recorder.RecordOverlapRejected(ctx, definitionID)
	}
	if stdErrors.Is(err, context.DeadlineExceeded) {
		m.recorder.RecordTimeout(ctx, definitionID)
	}
	if stdErrors.Is(err, backend.ErrLeaseAlreadyHeld) {
		m.recorder.RecordContention(ctx, definitionID)
	}
}

func recordBridgeAcquireFailure(m *Manager, re observe.Event, err error) {
	if stdErrors.Is(err, backend.ErrLeaseAlreadyHeld) {
		m.bridge.PublishRuntimeContention(re)
	}
	if stdErrors.Is(err, lockerrors.ErrOverlapRejected) {
		m.bridge.PublishRuntimeOverlapRejected(re)
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
	case stdErrors.Is(err, lockerrors.ErrOverlapRejected):
		return lockerrors.ErrOverlapRejected
	case stdErrors.Is(err, backend.ErrLeaseAlreadyHeld):
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
	count := int(m.activeCounter(definitionID).Load())
	m.recorder.RecordActiveLocks(ctx, definitionID, count)
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

func (m *Manager) buildAcquirePlan(def definitions.LockDefinition, input map[string]string) (runtimeAcquirePlan, error) {
	if !m.lineageDefs[def.ID] {
		resourceKey, err := def.KeyBuilder.Build(input)
		if err != nil {
			return runtimeAcquirePlan{}, err
		}
		return runtimeAcquirePlan{resourceKey: resourceKey}, nil
	}

	plan, err := lineage.ResolveAcquirePlan(def, m.cachedDefsByID, input)
	if err != nil {
		return runtimeAcquirePlan{}, err
	}
	meta := plan.LeaseMeta()
	return runtimeAcquirePlan{
		resourceKey: plan.ResourceKey,
		lineage:     &meta,
	}, nil
}

func (m *Manager) acquireLease(
	ctx context.Context,
	def definitions.LockDefinition,
	plan runtimeAcquirePlan,
	ownerID string,
) (heldLease, error) {
	if def.Mode == definitions.ModeStrict {
		if plan.lineage != nil {
			return heldLease{}, lockerrors.ErrPolicyViolation
		}
		strictDriver, ok := m.driver.(backend.StrictDriver)
		if !ok {
			return heldLease{}, lockerrors.ErrPolicyViolation
		}
		fenced, err := strictDriver.AcquireStrict(ctx, backend.StrictAcquireRequest{
			DefinitionID: def.ID,
			ResourceKey:  plan.resourceKey,
			OwnerID:      ownerID,
			LeaseTTL:     def.LeaseTTL,
		})
		if err != nil {
			return heldLease{}, err
		}
		return heldLease{
			lease:        fenced.Lease,
			fencingToken: fenced.FencingToken,
		}, nil
	}

	if plan.lineage == nil {
		lease, err := m.driver.Acquire(ctx, backend.AcquireRequest{
			DefinitionID: def.ID,
			ResourceKeys: []string{plan.resourceKey},
			OwnerID:      ownerID,
			LeaseTTL:     def.LeaseTTL,
		})
		if err != nil {
			return heldLease{}, err
		}
		return heldLease{lease: lease}, nil
	}

	lineageDriver, ok := m.driver.(backend.LineageDriver)
	if !ok {
		return heldLease{}, lockerrors.ErrPolicyViolation
	}

	lease, err := lineageDriver.AcquireWithLineage(ctx, backend.LineageAcquireRequest{
		DefinitionID: def.ID,
		ResourceKey:  plan.resourceKey,
		OwnerID:      ownerID,
		LeaseTTL:     def.LeaseTTL,
		Lineage:      cloneLineageMeta(*plan.lineage),
	})
	if err != nil {
		return heldLease{}, err
	}

	meta := cloneLineageMeta(*plan.lineage)
	return heldLease{
		lease:   lease,
		lineage: &meta,
	}, nil
}

func (m *Manager) releaseLease(ctx context.Context, held heldLease) error {
	if held.fencingToken > 0 {
		if held.lineage != nil {
			return lockerrors.ErrPolicyViolation
		}
		strictDriver, ok := m.driver.(backend.StrictDriver)
		if !ok {
			return lockerrors.ErrPolicyViolation
		}
		return strictDriver.ReleaseStrict(ctx, held.lease, held.fencingToken)
	}

	if held.lineage == nil {
		return m.driver.Release(ctx, held.lease)
	}

	lineageDriver, ok := m.driver.(backend.LineageDriver)
	if !ok {
		return lockerrors.ErrPolicyViolation
	}
	return lineageDriver.ReleaseWithLineage(ctx, held.lease, cloneLineageMeta(*held.lineage))
}

func cloneLineageMeta(meta backend.LineageLeaseMeta) backend.LineageLeaseMeta {
	out := meta
	if len(meta.AncestorKeys) > 0 {
		out.AncestorKeys = append([]backend.AncestorKey(nil), meta.AncestorKeys...)
	}
	return out
}
