package lockman

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/internal/sdk"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	lockregistry "github.com/tuanuet/lockman/lockkit/registry"
)

const defaultLeaseTTL = 30 * time.Second

type clientPlan struct {
	engineRegistry   *lockregistry.Registry
	hasRunUseCases   bool
	hasClaimUseCases bool
	hasHoldUseCases  bool
}

func buildClientPlan(cfg *clientConfig) (clientPlan, error) {
	if cfg.registry == nil {
		return clientPlan{}, ErrRegistryRequired
	}

	useCases := cfg.registry.registeredUseCases()
	if len(useCases) == 0 {
		if !hasStartupIdentity(cfg) {
			return clientPlan{}, ErrIdentityRequired
		}
		return clientPlan{
			engineRegistry: lockregistry.New(),
		}, nil
	}

	if !hasStartupIdentity(cfg) {
		return clientPlan{}, fmt.Errorf("lockman: configure WithIdentity(...) or WithIdentityProvider(...): %w", ErrIdentityRequired)
	}
	if cfg.backend == nil {
		return clientPlan{}, fmt.Errorf("lockman: configure WithBackend(...): %w", ErrBackendRequired)
	}

	childCounts := make(map[string]int, len(useCases))
	for _, useCase := range useCases {
		if useCase.kind == useCaseKindHold {
			if useCaseIsStrict(useCase) {
				return clientPlan{}, fmt.Errorf("lockman: hold use case %q does not support strict mode", useCase.name)
			}
			if len(useCase.config.composite) > 0 {
				return clientPlan{}, fmt.Errorf("lockman: hold use case %q does not support composite mode", useCase.name)
			}
		}
		if len(useCase.config.composite) > 0 && useCaseIsStrict(useCase) {
			return clientPlan{}, fmt.Errorf("lockman: composite use case %q does not support strict mode", useCase.name)
		}
		parentName := strings.TrimSpace(useCase.config.lineageParent)
		if parentName == "" {
			continue
		}
		childCounts[parentName]++
	}

	normalized := make([]sdk.UseCase, 0, len(useCases))
	normalizedByName := make(map[string]sdk.UseCase, len(useCases))
	for _, useCase := range useCases {
		norm := normalizeUseCase(useCase, childCounts, cfg.registry.link)
		normalized = append(normalized, norm)
		normalizedByName[useCase.name] = norm
	}

	if err := sdk.ValidateCapabilities(normalized, sdk.BackendCapabilities{
		HasIdempotencyStore: cfg.idempotency != nil,
		HasStrictBackend:    hasStrictBackend(cfg.backend),
		HasLineageBackend:   hasLineageBackend(cfg.backend),
	}); err != nil {
		return clientPlan{}, wrapCapabilityError(useCases, childCounts, err)
	}

	engineRegistry := lockregistry.New()
	plan := clientPlan{engineRegistry: engineRegistry}
	for _, useCase := range useCases {
		norm := normalizedByName[useCase.name]
		if err := registerEngineUseCase(engineRegistry, useCase, norm, normalizedByName); err != nil {
			return clientPlan{}, err
		}
		if useCase.kind == useCaseKindRun {
			plan.hasRunUseCases = true
		}
		if useCase.kind == useCaseKindClaim {
			plan.hasClaimUseCases = true
		}
		if useCase.kind == useCaseKindHold {
			plan.hasHoldUseCases = true
		}
	}

	return plan, nil
}

func registerEngineUseCase(
	engineRegistry *lockregistry.Registry,
	useCase *useCaseCore,
	normalized sdk.UseCase,
	normalizedByName map[string]sdk.UseCase,
) error {
	if len(useCase.config.composite) == 0 {
		def, err := translateUseCaseDefinition(useCase, normalized, normalizedByName)
		if err != nil {
			return err
		}
		if err := engineRegistry.Register(def); err != nil {
			return fmt.Errorf("lockman: register use case %q: %w", useCase.name, err)
		}
		return nil
	}

	memberIDs := make([]string, 0, len(useCase.config.composite))
	for _, member := range useCase.config.composite {
		def, err := translateCompositeMemberDefinition(useCase, normalized, member)
		if err != nil {
			return err
		}
		if err := engineRegistry.Register(def); err != nil {
			return fmt.Errorf("lockman: register composite member %q for use case %q: %w", member.name, useCase.name, err)
		}
		memberIDs = append(memberIDs, def.ID)
	}

	if err := engineRegistry.RegisterComposite(definitions.CompositeDefinition{
		ID:               normalized.DefinitionID(),
		Members:          memberIDs,
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   len(memberIDs),
		ExecutionKind:    definitions.ExecutionSync,
	}); err != nil {
		return fmt.Errorf("lockman: register composite use case %q: %w", useCase.name, err)
	}

	return nil
}

func hasStartupIdentity(cfg *clientConfig) bool {
	if cfg == nil {
		return false
	}
	if strings.TrimSpace(cfg.identity.OwnerID) != "" {
		return true
	}
	return cfg.identityProvider != nil
}

func useCaseIsStrict(useCase *useCaseCore) bool {
	if useCase.definition != nil {
		return useCase.definition.config.strict
	}
	if useCase.config.definitionRef != nil {
		return useCase.config.definitionRef.config.strict
	}
	return false
}

func normalizeUseCase(useCase *useCaseCore, childCounts map[string]int, link sdk.RegistryLink) sdk.UseCase {
	return sdk.NewUseCase(
		useCase.name,
		toSDKUseCaseKind(useCase.kind),
		sdk.CapabilityRequirements{
			RequiresIdempotency: useCase.kind == useCaseKindClaim && useCase.config.idempotent,
			RequiresStrict:      useCaseIsStrict(useCase),
			RequiresLineage:     strings.TrimSpace(useCase.config.lineageParent) != "" || childCounts[useCase.name] > 0,
		},
		link,
	)
}

func translateUseCaseDefinition(
	useCase *useCaseCore,
	normalized sdk.UseCase,
	normalizedByName map[string]sdk.UseCase,
) (definitions.LockDefinition, error) {
	ttl := useCase.config.ttl
	if ttl <= 0 {
		ttl = defaultLeaseTTL
	}

	definition := definitions.LockDefinition{
		ID:            normalized.DefinitionID(),
		Kind:          definitions.KindParent,
		Resource:      useCase.name,
		Mode:          definitions.ModeStandard,
		ExecutionKind: toExecutionKind(useCase.kind),
		LeaseTTL:      ttl,
		WaitTimeout:   useCase.config.wait,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("{"+sdk.ResourceKeyInputKey+"}", []string{sdk.ResourceKeyInputKey}),
	}

	if useCase.kind == useCaseKindClaim && useCase.config.idempotent {
		definition.IdempotencyRequired = true
	}
	if useCaseIsStrict(useCase) {
		definition.Mode = definitions.ModeStrict
		definition.FencingRequired = true
		definition.BackendFailurePolicy = definitions.BackendFailClosed
	}

	parentName := strings.TrimSpace(useCase.config.lineageParent)
	if parentName != "" {
		parent, ok := normalizedByName[parentName]
		if !ok {
			return definitions.LockDefinition{}, fmt.Errorf(
				"lockman: use case %q references unknown lineage parent %q: %w",
				useCase.name,
				parentName,
				ErrUseCaseNotFound,
			)
		}
		definition.Kind = definitions.KindChild
		definition.ParentRef = parent.DefinitionID()
		definition.OverlapPolicy = definitions.OverlapReject
	}

	return definition, nil
}

func translateCompositeMemberDefinition(
	useCase *useCaseCore,
	normalized sdk.UseCase,
	member compositeMemberConfig,
) (definitions.LockDefinition, error) {
	ttl := useCase.config.ttl
	if ttl <= 0 {
		ttl = defaultLeaseTTL
	}
	if member.name == "" {
		return definitions.LockDefinition{}, fmt.Errorf("lockman: composite member name is required for use case %q", useCase.name)
	}

	return definitions.LockDefinition{
		ID:            compositeMemberDefinitionID(normalized.DefinitionID(), member.name),
		Kind:          definitions.KindParent,
		Resource:      member.name,
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      ttl,
		WaitTimeout:   useCase.config.wait,
		Rank:          member.rank,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("{"+sdk.ResourceKeyInputKey+"}", []string{sdk.ResourceKeyInputKey}),
	}, nil
}

func compositeMemberDefinitionID(useCaseDefinitionID string, memberName string) string {
	return fmt.Sprintf("%s.member.%s", useCaseDefinitionID, strings.ReplaceAll(memberName, " ", "_"))
}

func toSDKUseCaseKind(kind useCaseKind) sdk.UseCaseKind {
	if kind == useCaseKindClaim {
		return sdk.UseCaseKindClaim
	}
	if kind == useCaseKindHold {
		return sdk.UseCaseKindHold
	}
	return sdk.UseCaseKindRun
}

func toExecutionKind(kind useCaseKind) definitions.ExecutionKind {
	if kind == useCaseKindClaim {
		return definitions.ExecutionAsync
	}
	return definitions.ExecutionSync
}

func hasStrictBackend(drv backend.Driver) bool {
	if drv == nil {
		return false
	}
	_, ok := drv.(backend.StrictDriver)
	return ok
}

func hasLineageBackend(drv backend.Driver) bool {
	if drv == nil {
		return false
	}
	_, ok := drv.(backend.LineageDriver)
	return ok
}

func wrapCapabilityError(useCases []*useCaseCore, childCounts map[string]int, err error) error {
	for _, useCase := range useCases {
		requiresLineage := strings.TrimSpace(useCase.config.lineageParent) != "" || childCounts[useCase.name] > 0
		switch {
		case errors.Is(err, sdk.ErrIdempotencyCapabilityRequired) && useCase.kind == useCaseKindClaim && useCase.config.idempotent:
			return fmt.Errorf("lockman: use case %q requires WithIdempotency(...): %w", useCase.name, ErrIdempotencyRequired)
		case errors.Is(err, sdk.ErrStrictCapabilityRequired) && useCaseIsStrict(useCase):
			return fmt.Errorf("lockman: use case %q requires strict backend support: %w", useCase.name, ErrBackendCapabilityRequired)
		case errors.Is(err, sdk.ErrLineageCapabilityRequired) && requiresLineage:
			return fmt.Errorf("lockman: use case %q requires lineage backend support: %w", useCase.name, ErrBackendCapabilityRequired)
		}
	}

	switch {
	case errors.Is(err, sdk.ErrIdempotencyCapabilityRequired):
		return fmt.Errorf("lockman: configure WithIdempotency(...): %w", ErrIdempotencyRequired)
	case errors.Is(err, sdk.ErrStrictCapabilityRequired):
		return fmt.Errorf("lockman: backend must implement strict locking: %w", ErrBackendCapabilityRequired)
	case errors.Is(err, sdk.ErrLineageCapabilityRequired):
		return fmt.Errorf("lockman: backend must implement lineage locking: %w", ErrBackendCapabilityRequired)
	default:
		return err
	}
}

func wrapStartupManagerError(component string, err error) error {
	switch {
	case errors.Is(err, lockerrors.ErrRegistryViolation):
		return fmt.Errorf("lockman: %s setup invalid: %w", component, ErrRegistryRequired)
	case errors.Is(err, lockerrors.ErrPolicyViolation):
		return fmt.Errorf("lockman: %s backend rejected startup configuration: %w", component, ErrBackendCapabilityRequired)
	default:
		return err
	}
}

func (c *Client) resolveIdentity(ctx context.Context, requestOwnerID string) (Identity, error) {
	if c == nil {
		return Identity{}, fmt.Errorf("lockman: client is nil")
	}

	identity := c.identity
	if c.identityProvider != nil {
		provided := c.identityProvider(ctx)
		if strings.TrimSpace(provided.OwnerID) != "" {
			identity.OwnerID = strings.TrimSpace(provided.OwnerID)
		}
		if strings.TrimSpace(provided.Service) != "" {
			identity.Service = strings.TrimSpace(provided.Service)
		}
		if strings.TrimSpace(provided.Instance) != "" {
			identity.Instance = strings.TrimSpace(provided.Instance)
		}
	}

	if trimmedOwner := strings.TrimSpace(requestOwnerID); trimmedOwner != "" {
		identity.OwnerID = trimmedOwner
	}
	identity.OwnerID = strings.TrimSpace(identity.OwnerID)
	identity.Service = strings.TrimSpace(identity.Service)
	identity.Instance = strings.TrimSpace(identity.Instance)

	if identity.OwnerID == "" {
		return Identity{}, fmt.Errorf("lockman: effective owner id is empty: %w", ErrIdentityRequired)
	}

	return identity, nil
}

func (c *Client) validateRunRequest(ctx context.Context, req RunRequest) (sdk.RunRequest, Identity, error) {
	if req.useCaseCore == nil {
		return sdk.RunRequest{}, Identity{}, ErrUseCaseNotFound
	}
	if !req.boundToRegistry {
		return sdk.RunRequest{}, Identity{}, fmt.Errorf("lockman: use case %q is not registered: %w", req.useCaseName, ErrUseCaseNotFound)
	}
	if c.registry == nil || sdk.RegistryLinkMismatch(c.registry.link, req.registryLink) {
		return sdk.RunRequest{}, Identity{}, fmt.Errorf("lockman: use case %q belongs to a different registry: %w", req.useCaseName, ErrRegistryMismatch)
	}

	identity, err := c.resolveIdentity(ctx, req.ownerID)
	if err != nil {
		return sdk.RunRequest{}, Identity{}, err
	}

	if len(req.compositeMemberInputs) > 0 {
		return sdk.RunRequest{}, identity, nil
	}

	return sdk.BindRunRequest(
		normalizeUseCase(req.useCaseCore, map[string]int{}, req.registryLink),
		req.resourceKey,
		identity.OwnerID,
	), identity, nil
}

func (c *Client) validateClaimRequest(ctx context.Context, req ClaimRequest) (sdk.ClaimRequest, Identity, error) {
	if req.useCaseCore == nil {
		return sdk.ClaimRequest{}, Identity{}, ErrUseCaseNotFound
	}
	if !req.boundToRegistry {
		return sdk.ClaimRequest{}, Identity{}, fmt.Errorf("lockman: use case %q is not registered: %w", req.useCaseName, ErrUseCaseNotFound)
	}
	if c.registry == nil || sdk.RegistryLinkMismatch(c.registry.link, req.registryLink) {
		return sdk.ClaimRequest{}, Identity{}, fmt.Errorf("lockman: use case %q belongs to a different registry: %w", req.useCaseName, ErrRegistryMismatch)
	}

	identity, err := c.resolveIdentity(ctx, req.ownerID)
	if err != nil {
		return sdk.ClaimRequest{}, Identity{}, err
	}

	return sdk.BindClaimRequest(
		normalizeUseCase(req.useCaseCore, map[string]int{}, req.registryLink),
		req.resourceKey,
		identity.OwnerID,
		sdk.ClaimDelivery{
			MessageID:     req.delivery.MessageID,
			ConsumerGroup: req.delivery.ConsumerGroup,
			Attempt:       req.delivery.Attempt,
		},
	), identity, nil
}

func (c *Client) validateHoldRequest(ctx context.Context, req HoldRequest) (Identity, error) {
	if req.useCaseCore == nil {
		return Identity{}, ErrUseCaseNotFound
	}
	if !req.boundToRegistry {
		return Identity{}, fmt.Errorf("lockman: use case %q is not registered: %w", req.useCaseName, ErrUseCaseNotFound)
	}
	if c.registry == nil || sdk.RegistryLinkMismatch(c.registry.link, req.registryLink) {
		return Identity{}, fmt.Errorf("lockman: use case %q belongs to a different registry: %w", req.useCaseName, ErrRegistryMismatch)
	}

	identity, err := c.resolveIdentity(ctx, req.ownerID)
	if err != nil {
		return Identity{}, err
	}

	return identity, nil
}

func (c *Client) validateForfeitRequest(req ForfeitRequest) error {
	if req.useCaseCore == nil {
		return ErrUseCaseNotFound
	}
	if !req.boundToRegistry {
		return fmt.Errorf("lockman: use case %q is not registered: %w", req.useCaseName, ErrUseCaseNotFound)
	}
	if c.registry == nil || sdk.RegistryLinkMismatch(c.registry.link, req.registryLink) {
		return fmt.Errorf("lockman: use case %q belongs to a different registry: %w", req.useCaseName, ErrRegistryMismatch)
	}

	return nil
}

func mapEngineError(err error, shuttingDown bool) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, lockerrors.ErrDuplicateIgnored):
		return ErrDuplicate
	case errors.Is(err, lockerrors.ErrOverlapRejected):
		return ErrOverlapRejected
	case errors.Is(err, lockerrors.ErrLockAcquireTimeout):
		return ErrTimeout
	case errors.Is(err, lockerrors.ErrLockBusy), errors.Is(err, lockerrors.ErrReentrantAcquire):
		return ErrBusy
	case errors.Is(err, lockerrors.ErrLeaseLost):
		return ErrLeaseLost
	case errors.Is(err, lockerrors.ErrInvariantRejected):
		return ErrInvariantRejected
	case errors.Is(err, lockerrors.ErrWorkerShuttingDown):
		return ErrShuttingDown
	case shuttingDown && errors.Is(err, lockerrors.ErrPolicyViolation):
		return ErrShuttingDown
	default:
		return err
	}
}

func joinErrors(left error, right error) error {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	return errors.Join(left, right)
}
