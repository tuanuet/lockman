package workers

import (
	"context"
	stdErrors "errors"
	"strings"
	"time"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	lockerrors "lockman/lockkit/errors"
	"lockman/lockkit/idempotency"
	"lockman/lockkit/internal/policy"
)

const (
	minInProgressTTL = 30 * time.Second
	minTerminalTTL   = time.Minute
	maxTerminalTTL   = 24 * time.Hour
)

// ExecuteClaimed runs fn after successfully acquiring a single-resource worker claim.
func (m *Manager) ExecuteClaimed(
	ctx context.Context,
	req definitions.MessageClaimRequest,
	fn func(context.Context, definitions.ClaimContext) error,
) (retErr error) {
	if fn == nil {
		return lockerrors.ErrPolicyViolation
	}
	if m.isShuttingDown() {
		return lockerrors.ErrWorkerShuttingDown
	}

	def, err := m.getDefinition(req.DefinitionID)
	if err != nil {
		return err
	}
	if err := validateClaimRequest(def, req); err != nil {
		return err
	}

	resourceKey, err := def.KeyBuilder.Build(req.KeyInput)
	if err != nil {
		return err
	}

	if !m.tryAdmitInFlightExecution() {
		return lockerrors.ErrWorkerShuttingDown
	}
	admitted := true
	defer func() {
		if admitted {
			m.releaseInFlightExecution()
		}
	}()

	guard := reentryKey{
		definitionID: def.ID,
		resourceKey:  resourceKey,
		ownerID:      req.Ownership.OwnerID,
	}
	if _, loaded := m.active.LoadOrStore(guard, struct{}{}); loaded {
		return lockerrors.ErrReentrantAcquire
	}
	guardInstalled := true
	defer func() {
		if guardInstalled {
			m.active.Delete(guard)
		}
	}()

	if err := m.preAcquireIdempotency(ctx, def, req); err != nil {
		return err
	}

	waitCfg, err := applyWorkerOverrides(def, req.Overrides)
	if err != nil {
		return err
	}

	acquireCtx, acquireCancel := contextWithAcquireTimeout(ctx, waitCfg)
	defer acquireCancel()

	lease, err := m.driver.Acquire(acquireCtx, drivers.AcquireRequest{
		DefinitionID: def.ID,
		ResourceKeys: []string{resourceKey},
		OwnerID:      req.Ownership.OwnerID,
		LeaseTTL:     def.LeaseTTL,
	})
	if err != nil {
		return mapAcquireError(err)
	}
	defer func() {
		_ = m.driver.Release(context.Background(), lease)
	}()

	callbackCtx, callbackCancel := context.WithCancel(ctx)
	defer callbackCancel()
	renewal := m.startLeaseRenewal(lease, callbackCancel)
	defer renewal.stopAndWait()

	callbackErr := fn(callbackCtx, definitions.ClaimContext{
		DefinitionID:   def.ID,
		ResourceKey:    resourceKey,
		Ownership:      req.Ownership,
		LeaseTTL:       lease.LeaseTTL,
		LeaseDeadline:  lease.ExpiresAt,
		IdempotencyKey: req.IdempotencyKey,
	})

	if renewErr := renewal.failure(); renewErr != nil {
		callbackErr = renewErr
	}

	outcome := policy.OutcomeFromError(callbackErr)
	if err := m.persistTerminalIdempotency(def, req, outcome); err != nil {
		if callbackErr == nil {
			callbackErr = err
		} else {
			callbackErr = stdErrors.Join(callbackErr, err)
		}
	}

	return callbackErr
}

func validateClaimRequest(def definitions.LockDefinition, req definitions.MessageClaimRequest) error {
	if strings.TrimSpace(req.DefinitionID) == "" {
		return lockerrors.ErrPolicyViolation
	}
	if def.ExecutionKind != definitions.ExecutionAsync && def.ExecutionKind != definitions.ExecutionBoth {
		return lockerrors.ErrPolicyViolation
	}
	if strings.TrimSpace(req.Ownership.OwnerID) == "" {
		return lockerrors.ErrPolicyViolation
	}
	if def.IdempotencyRequired && strings.TrimSpace(req.IdempotencyKey) == "" {
		return lockerrors.ErrPolicyViolation
	}
	return nil
}

func shouldUseIdempotency(def definitions.LockDefinition, req definitions.MessageClaimRequest) bool {
	if def.IdempotencyRequired {
		return true
	}
	return strings.TrimSpace(req.IdempotencyKey) != ""
}

func (m *Manager) preAcquireIdempotency(ctx context.Context, def definitions.LockDefinition, req definitions.MessageClaimRequest) error {
	if !shouldUseIdempotency(def, req) {
		return nil
	}
	if m.idempotency == nil {
		return lockerrors.ErrPolicyViolation
	}

	record, err := m.idempotency.Get(ctx, req.IdempotencyKey)
	if err != nil {
		return err
	}
	return m.handleIdempotencyRecord(ctx, def, req, record.Status)
}

func (m *Manager) handleIdempotencyRecord(
	ctx context.Context,
	def definitions.LockDefinition,
	req definitions.MessageClaimRequest,
	status idempotency.Status,
) error {
	switch status {
	case idempotency.StatusMissing:
		beginResult, err := m.idempotency.Begin(ctx, req.IdempotencyKey, idempotency.BeginInput{
			OwnerID:       req.Ownership.OwnerID,
			MessageID:     req.Ownership.MessageID,
			ConsumerGroup: req.Ownership.ConsumerGroup,
			Attempt:       req.Ownership.Attempt,
			TTL:           inProgressTTL(def.LeaseTTL),
		})
		if err != nil {
			return err
		}
		if beginResult.Acquired {
			return nil
		}
		return m.handleIdempotencyRecord(ctx, def, req, beginResult.Record.Status)
	case idempotency.StatusInProgress:
		return lockerrors.ErrLockBusy
	case idempotency.StatusCompleted:
		if err := m.idempotency.Complete(context.Background(), req.IdempotencyKey, idempotency.CompleteInput{
			OwnerID:   req.Ownership.OwnerID,
			MessageID: req.Ownership.MessageID,
			TTL:       terminalTTL(def.LeaseTTL),
		}); err != nil {
			return err
		}
		return lockerrors.ErrDuplicateIgnored
	case idempotency.StatusFailed:
		return lockerrors.ErrLockBusy
	default:
		return lockerrors.ErrLockBusy
	}
}

func (m *Manager) persistTerminalIdempotency(
	def definitions.LockDefinition,
	req definitions.MessageClaimRequest,
	outcome policy.WorkerOutcome,
) error {
	if !shouldUseIdempotency(def, req) {
		return nil
	}

	switch outcome {
	case policy.OutcomeAck:
		return m.idempotency.Complete(context.Background(), req.IdempotencyKey, idempotency.CompleteInput{
			OwnerID:   req.Ownership.OwnerID,
			MessageID: req.Ownership.MessageID,
			TTL:       terminalTTL(def.LeaseTTL),
		})
	case policy.OutcomeDrop, policy.OutcomeDLQ:
		return m.idempotency.Fail(context.Background(), req.IdempotencyKey, idempotency.FailInput{
			OwnerID:   req.Ownership.OwnerID,
			MessageID: req.Ownership.MessageID,
			TTL:       terminalTTL(def.LeaseTTL),
		})
	case policy.OutcomeRetry:
		return nil
	default:
		return nil
	}
}

func inProgressTTL(leaseTTL time.Duration) time.Duration {
	if leaseTTL <= 0 {
		return minInProgressTTL
	}
	ttl := leaseTTL * 2
	if ttl < minInProgressTTL {
		return minInProgressTTL
	}
	return ttl
}

func terminalTTL(leaseTTL time.Duration) time.Duration {
	if leaseTTL <= 0 {
		return minTerminalTTL
	}
	ttl := leaseTTL * 10
	if ttl < minTerminalTTL {
		return minTerminalTTL
	}
	if ttl > maxTerminalTTL {
		return maxTerminalTTL
	}
	return ttl
}

type waitConfig struct {
	timeout  time.Duration
	explicit bool
}

func applyWorkerOverrides(def definitions.LockDefinition, overrides *definitions.RuntimeOverrides) (waitConfig, error) {
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
