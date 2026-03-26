package runtime

import (
	"context"
	stdErrors "errors"
	"time"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	lockerrors "lockman/lockkit/errors"
	"lockman/lockkit/internal/policy"
)

type acquiredCompositeLease struct {
	member policy.MemberLeasePlan
	lease  drivers.LeaseRecord
}

// ExecuteCompositeExclusive runs fn after acquiring all composite members in canonical order.
func (m *Manager) ExecuteCompositeExclusive(
	ctx context.Context,
	req definitions.CompositeLockRequest,
	fn func(context.Context, definitions.LeaseContext) error,
) (retErr error) {
	if m.isShuttingDown() {
		return lockerrors.ErrPolicyViolation
	}

	compositeDef, err := m.getCompositeDefinition(req.DefinitionID)
	if err != nil {
		return err
	}
	if len(req.MemberInputs) != len(compositeDef.Members) {
		return lockerrors.ErrPolicyViolation
	}

	memberDefs := make([]definitions.LockDefinition, len(compositeDef.Members))
	memberKeys := make([]string, len(compositeDef.Members))
	for i, memberID := range compositeDef.Members {
		memberDef, memberErr := m.getDefinition(memberID)
		if memberErr != nil {
			return memberErr
		}

		memberKey, memberErr := memberDef.KeyBuilder.Build(req.MemberInputs[i])
		if memberErr != nil {
			return memberErr
		}

		memberDefs[i] = memberDef
		memberKeys[i] = memberKey
	}

	plan, err := policy.CanonicalizeMembers(memberDefs, memberKeys)
	if err != nil {
		return lockerrors.ErrPolicyViolation
	}
	if err := rejectCompositeOverlap(plan); err != nil {
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

	guardKeys := make([]guardKey, 0, len(plan))
	for _, member := range plan {
		key := guardKey{
			definitionID: member.Definition.ID,
			resourceKey:  member.ResourceKey,
			ownerID:      req.Ownership.OwnerID,
		}
		if _, loaded := m.active.LoadOrStore(key, guardEntry{state: guardPending}); loaded {
			return lockerrors.ErrReentrantAcquire
		}
		guardKeys = append(guardKeys, key)
	}
	guardInstalled := true
	defer func() {
		if !guardInstalled {
			return
		}
		for _, key := range guardKeys {
			m.active.Delete(key)
			m.recordActiveLocks(ctx, key.definitionID)
		}
	}()

	acquired := make([]acquiredCompositeLease, 0, len(plan))
	defer func() {
		for i := len(acquired) - 1; i >= 0; i-- {
			member := acquired[i]
			held := time.Since(member.lease.AcquiredAt)
			m.recorder.RecordRelease(ctx, member.member.Definition.ID, held)
			if releaseErr := m.driver.Release(context.Background(), member.lease); releaseErr != nil {
				if retErr == nil {
					retErr = releaseErr
				} else {
					retErr = stdErrors.Join(retErr, releaseErr)
				}
			}
		}
	}()

	for i, member := range plan {
		waitConfig, waitErr := applyRuntimeOverrides(member.Definition, req.Overrides)
		if waitErr != nil {
			return waitErr
		}

		acquireCtx, cancel := contextWithAcquireTimeout(ctx, waitConfig)
		start := time.Now()
		lease, acquireErr := m.driver.Acquire(acquireCtx, drivers.AcquireRequest{
			DefinitionID: member.Definition.ID,
			ResourceKeys: []string{member.ResourceKey},
			OwnerID:      req.Ownership.OwnerID,
			LeaseTTL:     member.Definition.LeaseTTL,
		})
		waitDuration := time.Since(start)
		cancel()

		m.recorder.RecordAcquire(ctx, member.Definition.ID, waitDuration, acquireErr == nil)
		if acquireErr != nil {
			recordAcquireFailure(m, ctx, member.Definition.ID, acquireErr)
			return mapAcquireError(acquireErr)
		}

		acquired = append(acquired, acquiredCompositeLease{
			member: member,
			lease:  lease,
		})
		m.active.Store(guardKeys[i], guardEntry{state: guardHeld})
		m.recordActiveLocks(ctx, member.Definition.ID)
	}

	retErr = fn(ctx, buildCompositeLeaseContext(req, acquired))
	return retErr
}

func rejectCompositeOverlap(plan []policy.MemberLeasePlan) error {
	for i := 0; i < len(plan); i++ {
		for j := i + 1; j < len(plan); j++ {
			left := plan[i]
			right := plan[j]

			if err := policy.RejectOverlap(left.Definition, right.Definition, left.ResourceKey, right.ResourceKey); err != nil {
				return err
			}
			if err := policy.RejectOverlap(right.Definition, left.Definition, right.ResourceKey, left.ResourceKey); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildCompositeLeaseContext(req definitions.CompositeLockRequest, acquired []acquiredCompositeLease) definitions.LeaseContext {
	resourceKeys := make([]string, len(acquired))
	var minTTL time.Duration
	var leaseDeadline time.Time

	for i, member := range acquired {
		resourceKeys[i] = member.member.ResourceKey
		if i == 0 || member.lease.LeaseTTL < minTTL {
			minTTL = member.lease.LeaseTTL
		}
		if i == 0 || member.lease.ExpiresAt.Before(leaseDeadline) {
			leaseDeadline = member.lease.ExpiresAt
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
