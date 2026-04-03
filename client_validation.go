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
	engineRegistry        *lockregistry.Registry
	hasRunUseCases        bool
	hasClaimUseCases      bool
	hasHoldUseCases       bool
	definitionIDByUseCase map[string]string
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

	plannedDefs, err := collectPlannedDefinitions(useCases, normalizedByName, cfg.registry.link)
	if err != nil {
		return clientPlan{}, err
	}

	engineRegistry := lockregistry.New()
	definitionIDByUseCase := make(map[string]string, len(useCases))
	for _, def := range plannedDefs {
		if err := engineRegistry.Register(def.definition); err != nil {
			return clientPlan{}, fmt.Errorf("lockman: register definition %q: %w", def.definition.ID, err)
		}
		for _, ucName := range def.useCaseNames {
			definitionIDByUseCase[ucName] = def.definition.ID
		}
	}

	for _, useCase := range useCases {
		if resolveDefinitionID(useCase) != "" {
			continue
		}
		norm := normalizedByName[useCase.name]
		if len(useCase.config.composite) == 0 {
			def, err := translateUseCaseDefinition(useCase, norm, normalizedByName)
			if err != nil {
				return clientPlan{}, err
			}
			if err := engineRegistry.Register(def); err != nil {
				return clientPlan{}, fmt.Errorf("lockman: register use case %q: %w", useCase.name, err)
			}
			definitionIDByUseCase[useCase.name] = def.ID
			continue
		}

		memberIDs := make([]string, 0, len(useCase.config.composite))
		for _, member := range useCase.config.composite {
			def, err := translateCompositeMemberDefinition(useCase, norm, member)
			if err != nil {
				return clientPlan{}, err
			}
			if err := engineRegistry.Register(def); err != nil {
				return clientPlan{}, fmt.Errorf("lockman: register composite member %q for use case %q: %w", member.name, useCase.name, err)
			}
			memberIDs = append(memberIDs, def.ID)
		}

		if err := engineRegistry.RegisterComposite(definitions.CompositeDefinition{
			ID:               norm.DefinitionID(),
			Members:          memberIDs,
			OrderingPolicy:   definitions.OrderingCanonical,
			AcquirePolicy:    definitions.AcquireAllOrNothing,
			EscalationPolicy: definitions.EscalationReject,
			ModeResolution:   definitions.ModeResolutionHomogeneous,
			MaxMemberCount:   len(memberIDs),
			ExecutionKind:    definitions.ExecutionSync,
		}); err != nil {
			return clientPlan{}, fmt.Errorf("lockman: register composite use case %q: %w", useCase.name, err)
		}
		definitionIDByUseCase[useCase.name] = norm.DefinitionID()
	}

	plan := clientPlan{
		engineRegistry:        engineRegistry,
		definitionIDByUseCase: definitionIDByUseCase,
	}
	for _, useCase := range useCases {
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

type plannedDefinition struct {
	definition   definitions.LockDefinition
	useCaseNames []string
}

func collectPlannedDefinitions(
	useCases []*useCaseCore,
	normalizedByName map[string]sdk.UseCase,
	link sdk.RegistryLink,
) (map[string]plannedDefinition, error) {
	definitionKinds := make(map[string]map[useCaseKind]bool)
	definitionUseCases := make(map[string][]*useCaseCore)
	definitionStrict := make(map[string]bool)
	definitionTTLValues := make(map[string]map[time.Duration][]string)
	definitionWaitValues := make(map[string]map[time.Duration][]string)
	definitionIdempotent := make(map[string]bool)
	definitionLineage := make(map[string]string)

	for _, uc := range useCases {
		defID := resolveDefinitionID(uc)
		if defID == "" {
			continue
		}

		if definitionKinds[defID] == nil {
			definitionKinds[defID] = make(map[useCaseKind]bool)
		}
		definitionKinds[defID][uc.kind] = true
		definitionUseCases[defID] = append(definitionUseCases[defID], uc)

		if useCaseIsStrict(uc) {
			definitionStrict[defID] = true
		}
		if uc.config.ttl > 0 {
			if definitionTTLValues[defID] == nil {
				definitionTTLValues[defID] = make(map[time.Duration][]string)
			}
			definitionTTLValues[defID][uc.config.ttl] = append(definitionTTLValues[defID][uc.config.ttl], uc.name)
		}
		if uc.config.wait > 0 {
			if definitionWaitValues[defID] == nil {
				definitionWaitValues[defID] = make(map[time.Duration][]string)
			}
			definitionWaitValues[defID][uc.config.wait] = append(definitionWaitValues[defID][uc.config.wait], uc.name)
		}
		if uc.kind == useCaseKindClaim && uc.config.idempotent {
			definitionIdempotent[defID] = true
		}
		if parent := strings.TrimSpace(uc.config.lineageParent); parent != "" {
			definitionLineage[defID] = parent
		}
	}

	result := make(map[string]plannedDefinition)
	for defID, kinds := range definitionKinds {
		execKind := executionKindForDefinition(kinds)
		ucList := definitionUseCases[defID]
		useCaseNames := make([]string, 0, len(ucList))
		for _, uc := range ucList {
			useCaseNames = append(useCaseNames, uc.name)
		}

		ttl, err := resolveDefinitionOption("TTL", defID, useCaseNames, definitionTTLValues[defID])
		if err != nil {
			return nil, err
		}
		if ttl <= 0 {
			ttl = defaultLeaseTTL
		}

		wait, err := resolveDefinitionOption("WaitTimeout", defID, useCaseNames, definitionWaitValues[defID])
		if err != nil {
			return nil, err
		}

		strict := definitionStrict[defID]
		if strict && len(kinds) == 1 {
			if _, hasHold := kinds[useCaseKindHold]; hasHold {
				return nil, fmt.Errorf("lockman: hold use case %q does not support strict mode", ucList[0].name)
			}
		}

		resourceName := useCaseNames[0]
		if len(useCaseNames) > 1 {
			resourceName = defID
		}

		def := definitions.LockDefinition{
			ID:            defID,
			Kind:          definitions.KindParent,
			Resource:      resourceName,
			Mode:          definitions.ModeStandard,
			ExecutionKind: execKind,
			LeaseTTL:      ttl,
			WaitTimeout:   wait,
			KeyBuilder:    definitions.MustTemplateKeyBuilder("{"+sdk.ResourceKeyInputKey+"}", []string{sdk.ResourceKeyInputKey}),
		}

		if definitionIdempotent[defID] {
			def.IdempotencyRequired = true
		}
		if strict {
			def.Mode = definitions.ModeStrict
			def.FencingRequired = true
			def.BackendFailurePolicy = definitions.BackendFailClosed
		}

		if parentName := definitionLineage[defID]; parentName != "" {
			parent, ok := normalizedByName[parentName]
			if !ok {
				return nil, fmt.Errorf(
					"lockman: use case %q references unknown lineage parent %q: %w",
					ucList[0].name,
					parentName,
					ErrUseCaseNotFound,
				)
			}
			def.Kind = definitions.KindChild
			def.ParentRef = parent.DefinitionID()
			def.OverlapPolicy = definitions.OverlapReject
		}

		result[defID] = plannedDefinition{
			definition:   def,
			useCaseNames: useCaseNames,
		}
	}

	return result, nil
}

func resolveDefinitionOption(
	optionName string,
	defID string,
	allUseCaseNames []string,
	values map[time.Duration][]string,
) (time.Duration, error) {
	if len(values) == 0 {
		return 0, nil
	}
	if len(values) == 1 {
		for v := range values {
			return v, nil
		}
	}

	conflicting := make([]string, 0, len(values)*2)
	for v, names := range values {
		for _, n := range names {
			conflicting = append(conflicting, fmt.Sprintf("%s (%v)", n, v))
		}
	}
	return 0, fmt.Errorf("lockman: shared definition %q has conflicting %s values across use cases: %s", defID, optionName, strings.Join(conflicting, ", "))
}

func executionKindForDefinition(kinds map[useCaseKind]bool) definitions.ExecutionKind {
	hasRun := kinds[useCaseKindRun]
	hasClaim := kinds[useCaseKindClaim]
	hasHold := kinds[useCaseKindHold]

	if hasClaim {
		return definitions.ExecutionBoth
	}
	if hasRun || hasHold {
		return definitions.ExecutionSync
	}
	return definitions.ExecutionSync
}

func resolveDefinitionID(uc *useCaseCore) string {
	if uc.definition != nil {
		return uc.definition.id
	}
	if uc.config.definitionRef != nil {
		return uc.config.definitionRef.id
	}
	return ""
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
	defID := resolveDefinitionID(useCase)
	if defID != "" {
		return sdk.NewUseCaseWithID(
			useCase.name,
			defID,
			toSDKUseCaseKind(useCase.kind),
			sdk.CapabilityRequirements{
				RequiresIdempotency: useCase.kind == useCaseKindClaim && useCase.config.idempotent,
				RequiresStrict:      useCaseIsStrict(useCase),
				RequiresLineage:     strings.TrimSpace(useCase.config.lineageParent) != "" || childCounts[useCase.name] > 0,
			},
			link,
		)
	}
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
