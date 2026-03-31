package runtime

import (
	"context"
	stdErrors "errors"
	"time"

	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/internal/policy"
)

type acquiredCompositeLease struct {
	member policy.MemberLeasePlan
	held   heldLease
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
	memberPlans := make(map[compositePlanKey][]runtimeAcquirePlan, len(compositeDef.Members))
	for i, memberID := range compositeDef.Members {
		memberDef, memberErr := m.getDefinition(memberID)
		if memberErr != nil {
			return memberErr
		}

		acquirePlan, memberErr := m.buildAcquirePlan(memberDef, req.MemberInputs[i])
		if memberErr != nil {
			return memberErr
		}

		memberDefs[i] = memberDef
		memberKeys[i] = acquirePlan.resourceKey
		key := compositePlanKey{definitionID: memberDef.ID, resourceKey: acquirePlan.resourceKey}
		memberPlans[key] = append(memberPlans[key], acquirePlan)
	}

	plan, err := policy.CanonicalizeMembers(memberDefs, memberKeys)
	if err != nil {
		return lockerrors.ErrPolicyViolation
	}
	if err := rejectCompositeOverlap(plan); err != nil {
		return err
	}

	waitConfigs := make([]waitConfig, len(plan))
	for i, member := range plan {
		cfg, waitErr := applyRuntimeOverrides(member.Definition, req.Overrides)
		if waitErr != nil {
			return waitErr
		}
		waitConfigs[i] = cfg
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
			held := time.Since(member.held.lease.AcquiredAt)
			m.recorder.RecordRelease(ctx, member.member.Definition.ID, held)
			if m.bridge != nil {
				m.bridge.PublishRuntimeReleased(RuntimeEvent{
					DefinitionID: member.member.Definition.ID,
					ResourceID:   member.member.ResourceKey,
					OwnerID:      req.Ownership.OwnerID,
					RequestID:    req.Ownership.RequestID,
					Held:         held,
				})
			}
			if releaseErr := m.releaseLease(context.Background(), member.held); releaseErr != nil {
				if retErr == nil {
					retErr = releaseErr
				} else {
					retErr = stdErrors.Join(retErr, releaseErr)
				}
			}
		}
	}()

	for i, member := range plan {
		acquirePlan, ok := popCompositeAcquirePlan(memberPlans, member.Definition.ID, member.ResourceKey)
		if !ok {
			return lockerrors.ErrPolicyViolation
		}

		acquireCtx, cancel := contextWithAcquireTimeout(ctx, waitConfigs[i])
		re := RuntimeEvent{
			DefinitionID: member.Definition.ID,
			ResourceID:   member.ResourceKey,
			OwnerID:      req.Ownership.OwnerID,
			RequestID:    req.Ownership.RequestID,
		}
		if m.bridge != nil {
			m.bridge.PublishRuntimeAcquireStarted(re)
		}
		start := time.Now()
		lease, acquireErr := m.acquireLease(acquireCtx, member.Definition, acquirePlan, req.Ownership.OwnerID)
		waitDuration := time.Since(start)
		cancel()

		re.Wait = waitDuration
		m.recorder.RecordAcquire(ctx, member.Definition.ID, waitDuration, acquireErr == nil)
		if acquireErr != nil {
			recordAcquireFailure(m, ctx, member.Definition.ID, acquireErr)
			if m.bridge != nil {
				m.bridge.PublishRuntimeAcquireFailed(re, acquireErr)
				recordBridgeAcquireFailure(m, re, acquireErr)
			}
			return mapAcquireError(acquireErr)
		}

		if m.bridge != nil {
			m.bridge.PublishRuntimeAcquireSucceeded(re)
		}

		acquired = append(acquired, acquiredCompositeLease{
			member: member,
			held:   lease,
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
		if i == 0 || member.held.lease.LeaseTTL < minTTL {
			minTTL = member.held.lease.LeaseTTL
		}
		if i == 0 || member.held.lease.ExpiresAt.Before(leaseDeadline) {
			leaseDeadline = member.held.lease.ExpiresAt
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

type compositePlanKey struct {
	definitionID string
	resourceKey  string
}

func popCompositeAcquirePlan(
	plans map[compositePlanKey][]runtimeAcquirePlan,
	definitionID string,
	resourceKey string,
) (runtimeAcquirePlan, bool) {
	key := compositePlanKey{definitionID: definitionID, resourceKey: resourceKey}
	queue := plans[key]
	if len(queue) == 0 {
		return runtimeAcquirePlan{}, false
	}
	plan := queue[0]
	if len(queue) == 1 {
		delete(plans, key)
	} else {
		plans[key] = queue[1:]
	}
	return plan, true
}
