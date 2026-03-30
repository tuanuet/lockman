package workers

import (
	"context"
	stdErrors "errors"
	"strings"
	"time"

	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/internal/policy"
)

type acquiredCompositeClaim struct {
	member policy.MemberLeasePlan
	lease  renewableLease
}

// ExecuteCompositeClaimed runs fn after successfully acquiring all composite members in canonical order.
func (m *Manager) ExecuteCompositeClaimed(
	ctx context.Context,
	req definitions.CompositeClaimRequest,
	fn func(context.Context, definitions.ClaimContext) error,
) (retErr error) {
	if fn == nil {
		return lockerrors.ErrPolicyViolation
	}
	if m.isShuttingDown() {
		return lockerrors.ErrWorkerShuttingDown
	}
	if err := validateCompositeClaimRequest(req); err != nil {
		return err
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
	memberPlans := make(map[compositeClaimPlanKey][]claimAcquirePlan, len(compositeDef.Members))
	idempotencyRequired := false
	idempotencyLeaseTTLBasis := time.Duration(0)
	for i, memberID := range compositeDef.Members {
		memberDef, memberErr := m.getDefinition(memberID)
		if memberErr != nil {
			return memberErr
		}
		if memberDef.Mode != definitions.ModeStandard {
			return lockerrors.ErrPolicyViolation
		}
		if memberDef.ExecutionKind != definitions.ExecutionAsync && memberDef.ExecutionKind != definitions.ExecutionBoth {
			return lockerrors.ErrPolicyViolation
		}

		acquirePlan, memberErr := m.buildClaimAcquirePlan(memberDef, req.MemberInputs[i])
		if memberErr != nil {
			return memberErr
		}

		memberDefs[i] = memberDef
		memberKeys[i] = acquirePlan.resourceKey
		key := compositeClaimPlanKey{definitionID: memberDef.ID, resourceKey: acquirePlan.resourceKey}
		memberPlans[key] = append(memberPlans[key], acquirePlan)
		idempotencyRequired = idempotencyRequired || memberDef.IdempotencyRequired
		if i == 0 || memberDef.LeaseTTL > idempotencyLeaseTTLBasis {
			idempotencyLeaseTTLBasis = memberDef.LeaseTTL
		}
	}

	execDef := definitions.LockDefinition{
		ID:                  compositeDef.ID,
		ExecutionKind:       compositeDef.ExecutionKind,
		IdempotencyRequired: idempotencyRequired,
		// Phase 2 idempotency retention is reject-first and should conservatively
		// cover the longest-held composite member lease.
		LeaseTTL: idempotencyLeaseTTLBasis,
	}
	msgReq := definitions.MessageClaimRequest{
		DefinitionID:   req.DefinitionID,
		Ownership:      req.Ownership,
		IdempotencyKey: req.IdempotencyKey,
		Overrides:      req.Overrides,
	}
	if err := validateClaimRequest(execDef, msgReq); err != nil {
		return err
	}

	plan, err := policy.CanonicalizeMembers(memberDefs, memberKeys)
	if err != nil {
		return lockerrors.ErrPolicyViolation
	}
	if err := rejectCompositeOverlap(plan); err != nil {
		return err
	}

	waitCfgs := make([]waitConfig, len(plan))
	for i, member := range plan {
		cfg, waitErr := applyWorkerOverrides(member.Definition, req.Overrides)
		if waitErr != nil {
			return waitErr
		}
		waitCfgs[i] = cfg
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

	guardKeys := make([]reentryKey, 0, len(plan))
	for _, member := range plan {
		guard := reentryKey{
			definitionID: member.Definition.ID,
			resourceKey:  member.ResourceKey,
		}
		if _, loaded := m.active.LoadOrStore(guard, struct{}{}); loaded {
			return lockerrors.ErrReentrantAcquire
		}
		guardKeys = append(guardKeys, guard)
	}
	guardInstalled := true
	defer func() {
		if !guardInstalled {
			return
		}
		for _, key := range guardKeys {
			m.active.Delete(key)
		}
	}()

	if err := m.preAcquireIdempotency(ctx, execDef, msgReq); err != nil {
		return err
	}

	acquired := make([]acquiredCompositeClaim, 0, len(plan))
	defer func() {
		for i := len(acquired) - 1; i >= 0; i-- {
			_ = m.releaseClaimLease(context.Background(), acquired[i].lease)
		}
	}()

	for i, member := range plan {
		acquirePlan, ok := popCompositeClaimAcquirePlan(memberPlans, member.Definition.ID, member.ResourceKey)
		if !ok {
			return lockerrors.ErrPolicyViolation
		}

		acquireCtx, acquireCancel := contextWithAcquireTimeout(ctx, waitCfgs[i])
		lease, acquireErr := m.acquireClaimLease(acquireCtx, member.Definition, acquirePlan, req.Ownership.OwnerID)
		acquireCancel()
		if acquireErr != nil {
			return mapAcquireError(acquireErr)
		}

		acquired = append(acquired, acquiredCompositeClaim{
			member: member,
			lease:  lease,
		})
	}

	callbackCtx, callbackCancel := context.WithCancel(ctx)
	defer callbackCancel()
	renewal := startCompositeLeaseRenewal(m, acquired, callbackCancel)
	renewalStopped := false
	defer func() {
		if !renewalStopped {
			renewal.stopAndWait()
		}
	}()

	callbackErr := fn(callbackCtx, buildCompositeClaimContext(req, acquired))

	renewal.stopAndWait()
	renewalStopped = true

	if renewErr := renewal.failure(); renewErr != nil {
		callbackErr = renewErr
	}

	outcome := policy.OutcomeFromError(callbackErr)
	if err := m.persistTerminalIdempotency(execDef, msgReq, outcome); err != nil {
		if callbackErr == nil {
			callbackErr = err
		} else {
			callbackErr = stdErrors.Join(callbackErr, err)
		}
	}

	return callbackErr
}

type compositeRenewalSession struct {
	sessions []*renewalSession
}

func startCompositeLeaseRenewal(
	m *Manager,
	acquired []acquiredCompositeClaim,
	onFailureCancel context.CancelFunc,
) *compositeRenewalSession {
	sessions := make([]*renewalSession, 0, len(acquired))
	for _, member := range acquired {
		sessions = append(sessions, m.startLeaseRenewal(member.lease, onFailureCancel))
	}
	return &compositeRenewalSession{sessions: sessions}
}

func (s *compositeRenewalSession) stopAndWait() {
	if s == nil {
		return
	}
	for i := len(s.sessions) - 1; i >= 0; i-- {
		s.sessions[i].stopAndWait()
	}
}

func (s *compositeRenewalSession) failure() error {
	if s == nil {
		return nil
	}
	var err error
	for _, session := range s.sessions {
		if renewErr := session.failure(); renewErr != nil {
			if err == nil {
				err = renewErr
			} else {
				err = stdErrors.Join(err, renewErr)
			}
		}
	}
	return err
}

func buildCompositeClaimContext(
	req definitions.CompositeClaimRequest,
	acquired []acquiredCompositeClaim,
) definitions.ClaimContext {
	resourceKeys := make([]string, len(acquired))
	var minTTL time.Duration
	var leaseDeadline time.Time

	for i, member := range acquired {
		resourceKeys[i] = member.member.ResourceKey
		if i == 0 || member.lease.lease.LeaseTTL < minTTL {
			minTTL = member.lease.lease.LeaseTTL
		}
		if i == 0 || member.lease.lease.ExpiresAt.Before(leaseDeadline) {
			leaseDeadline = member.lease.lease.ExpiresAt
		}
	}

	return definitions.ClaimContext{
		DefinitionID:   req.DefinitionID,
		ResourceKeys:   resourceKeys,
		Ownership:      req.Ownership,
		LeaseTTL:       minTTL,
		LeaseDeadline:  leaseDeadline,
		IdempotencyKey: req.IdempotencyKey,
	}
}

// rejectCompositeOverlap enforces Phase 2 reject-first overlap behavior.
// Escalation is intentionally out of scope for composite worker execution.
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

func (m *Manager) getCompositeDefinition(id string) (def definitions.CompositeDefinition, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = lockerrors.ErrPolicyViolation
		}
	}()
	def = m.registry.MustGetComposite(id)
	return def, err
}

func validateCompositeClaimRequest(req definitions.CompositeClaimRequest) error {
	if strings.TrimSpace(req.DefinitionID) == "" {
		return lockerrors.ErrPolicyViolation
	}
	if strings.TrimSpace(req.Ownership.OwnerID) == "" {
		return lockerrors.ErrPolicyViolation
	}
	return nil
}

type compositeClaimPlanKey struct {
	definitionID string
	resourceKey  string
}

func popCompositeClaimAcquirePlan(
	plans map[compositeClaimPlanKey][]claimAcquirePlan,
	definitionID string,
	resourceKey string,
) (claimAcquirePlan, bool) {
	key := compositeClaimPlanKey{definitionID: definitionID, resourceKey: resourceKey}
	queue := plans[key]
	if len(queue) == 0 {
		return claimAcquirePlan{}, false
	}
	plan := queue[0]
	if len(queue) == 1 {
		delete(plans, key)
	} else {
		plans[key] = queue[1:]
	}
	return plan, true
}
