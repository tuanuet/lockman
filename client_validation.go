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
	lineageDefinitionIDs  map[string]bool
}

func findDefinitionStrict(reg *Registry, id string) bool {
	if reg == nil {
		return false
	}
	for _, defRef := range reg.findDefinitionRefs() {
		if defRef.id == id || defRef.name == id {
			return defRef.config.strict
		}
	}
	return false
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
		if len(useCase.config.composite) > 0 {
			if useCaseIsStrict(useCase) {
				return clientPlan{}, fmt.Errorf("lockman: composite use case %q does not support strict mode", useCase.name)
			}
			// Check if any composite member is from a strict definition
			for _, member := range useCase.config.composite {
				if member.memberIsStrict {
					return clientPlan{}, fmt.Errorf("lockman: composite use case %q does not support strict member %q", useCase.name, member.name)
				}
			}
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

	plannedDefs, err := collectPlannedDefinitions(useCases, normalizedByName, cfg.registry.link, cfg.registry)
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
		hasStrictMember := false
		for _, member := range useCase.config.composite {
			if member.definitionID != "" {
				// Check if this member definition is strict by checking any use case that uses this definition
				// If any use case with this definition ID is strict, then it's a strict definition
				if isDefinitionStrict(useCases, member.definitionID) {
					hasStrictMember = true
					break
				}
				memberIDs = append(memberIDs, member.definitionID)
				continue
			}

			def, err := translateCompositeMemberDefinition(useCase, norm, member)
			if err != nil {
				return clientPlan{}, err
			}
			if err := engineRegistry.Register(def); err != nil {
				return clientPlan{}, fmt.Errorf("lockman: register composite member %q for use case %q: %w", member.name, useCase.name, err)
			}
			memberIDs = append(memberIDs, def.ID)
		}

		if hasStrictMember {
			return clientPlan{}, fmt.Errorf("lockman: composite use case %q does not support strict members", useCase.name)
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
		lineageDefinitionIDs:  lineageDefinitionIDs(engineRegistry.Definitions()),
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

func lineageDefinitionIDs(defs []definitions.LockDefinition) map[string]bool {
	if len(defs) == 0 {
		return map[string]bool{}
	}

	parentByID := make(map[string]string, len(defs))
	marked := make(map[string]bool, len(defs))
	for _, def := range defs {
		parent := strings.TrimSpace(def.ParentRef)
		parentByID[def.ID] = parent
		if parent != "" {
			marked[def.ID] = true
		}
	}

	changed := true
	for changed {
		changed = false
		for id, isMarked := range marked {
			if !isMarked {
				continue
			}
			parent := parentByID[id]
			if parent == "" || marked[parent] {
				continue
			}
			marked[parent] = true
			changed = true
		}
	}

	return marked
}

type plannedDefinition struct {
	definition   definitions.LockDefinition
	useCaseNames []string
}

func collectPlannedDefinitions(
	useCases []*useCaseCore,
	normalizedByName map[string]sdk.UseCase,
	link sdk.RegistryLink,
	reg *Registry,
) (map[string]plannedDefinition, error) {
	definitionKinds := make(map[string]map[useCaseKind]bool)
	definitionUseCases := make(map[string][]*useCaseCore)
	definitionStrict := make(map[string]bool)
	definitionTTLValues := make(map[string]map[time.Duration][]string)
	definitionWaitValues := make(map[string]map[time.Duration][]string)
	definitionIdempotent := make(map[string]bool)
	definitionLineage := make(map[string]string)
	definitionFailIfHeld := make(map[string]bool)

	// Build a map of definition names to their strictness from all definitions in the system
	// This includes both top-level use case definitions and composite member definitions
	allDefinitionsStrict := make(map[string]bool)
	for _, uc := range useCases {
		defID := resolveDefinitionID(uc)
		if defID != "" && useCaseIsStrict(uc) {
			allDefinitionsStrict[defID] = true
		}
		// For composite use cases, also check if the composite use case itself or its members are strict
		for _, member := range uc.config.composite {
			if member.definitionID != "" && useCaseIsStrict(uc) {
				allDefinitionsStrict[member.definitionID] = true
			}
		}
	}

	// First pass: collect all definition IDs from composite member references
	for _, uc := range useCases {
		defID := resolveDefinitionID(uc)
		if defID == "" {
			for _, member := range uc.config.composite {
				if member.definitionID != "" {
					if definitionKinds[member.definitionID] == nil {
						definitionKinds[member.definitionID] = make(map[useCaseKind]bool)
					}
					definitionKinds[member.definitionID][useCaseKindRun] = true
				}
			}
			continue
		}
		if definitionKinds[defID] == nil {
			definitionKinds[defID] = make(map[useCaseKind]bool)
		}
	}

	// Second pass: determine strictness from all definitions
	for _, uc := range useCases {
		defID := resolveDefinitionID(uc)
		if defID != "" {
			if useCaseIsStrict(uc) {
				definitionStrict[defID] = true
			}
			continue
		}
		for _, member := range uc.config.composite {
			if member.definitionID != "" {
				if useCaseIsStrict(uc) {
					definitionStrict[member.definitionID] = true
				}
			}
		}
	}

	// Third pass: collect use case associations and other metadata
	for _, uc := range useCases {
		defID := resolveDefinitionID(uc)
		if defID == "" {
			for _, member := range uc.config.composite {
				if member.definitionID != "" {
					definitionUseCases[member.definitionID] = append(definitionUseCases[member.definitionID], uc)
					if uc.config.ttl > 0 {
						if definitionTTLValues[member.definitionID] == nil {
							definitionTTLValues[member.definitionID] = make(map[time.Duration][]string)
						}
						definitionTTLValues[member.definitionID][uc.config.ttl] = append(definitionTTLValues[member.definitionID][uc.config.ttl], uc.name)
					}
					if uc.config.wait > 0 {
						if definitionWaitValues[member.definitionID] == nil {
							definitionWaitValues[member.definitionID] = make(map[time.Duration][]string)
						}
						definitionWaitValues[member.definitionID][uc.config.wait] = append(definitionWaitValues[member.definitionID][uc.config.wait], uc.name)
					}
					if member.failIfHeld {
						definitionFailIfHeld[member.definitionID] = true
					}
				}
			}
			continue
		}

		if definitionKinds[defID] == nil {
			definitionKinds[defID] = make(map[useCaseKind]bool)
		}
		definitionKinds[defID][uc.kind] = true
		definitionUseCases[defID] = append(definitionUseCases[defID], uc)

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
			Kind:          backend.KindParent,
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
		if definitionFailIfHeld[defID] {
			def.FailIfHeld = true
			def.CheckOnlyAllowed = true
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
			def.Kind = backend.KindChild
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

	if hasRun && hasClaim {
		return definitions.ExecutionBoth
	}

	if hasClaim {
		return definitions.ExecutionBoth
	}
	if hasRun || hasHold {
		return definitions.ExecutionSync
	}
	return definitions.ExecutionSync
}

func isDefinitionStrict(useCases []*useCaseCore, definitionID string) bool {
	for _, uc := range useCases {
		// Check composite members
		for _, member := range uc.config.composite {
			if member.definitionID == definitionID {
				// Check if this member is from a strict definition
				if isUseCaseUsingStrictDefinition(useCases, uc, definitionID) {
					return true
				}
			}
		}
	}
	return false
}

func isUseCaseUsingStrictDefinition(useCases []*useCaseCore, uc *useCaseCore, definitionID string) bool {
	if useCaseIsStrict(uc) {
		return true
	}
	for _, otherUC := range useCases {
		otherDefID := resolveDefinitionID(otherUC)
		if otherDefID == definitionID && useCaseIsStrict(otherUC) {
			return true
		}
		for _, member := range otherUC.config.composite {
			if member.definitionID == definitionID && useCaseIsStrict(otherUC) {
				return true
			}
		}
	}
	return false
}

func collectCompositeMemberDefinitions(useCases []*useCaseCore) map[string]bool {
	result := make(map[string]bool)
	for _, uc := range useCases {
		defID := resolveDefinitionID(uc)
		if defID != "" && useCaseIsStrict(uc) {
			result[defID] = true
		}
		for _, member := range uc.config.composite {
			if member.definitionID != "" && useCaseIsStrict(uc) {
				result[member.definitionID] = true
			}
		}
	}
	return result
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
		Kind:          backend.KindParent,
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
		definition.Kind = backend.KindChild
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
		ID:               compositeMemberDefinitionID(normalized.DefinitionID(), member.name),
		Kind:             backend.KindParent,
		Resource:         member.name,
		Mode:             definitions.ModeStandard,
		ExecutionKind:    definitions.ExecutionSync,
		LeaseTTL:         ttl,
		WaitTimeout:      useCase.config.wait,
		Rank:             member.rank,
		FailIfHeld:       member.failIfHeld,
		CheckOnlyAllowed: member.failIfHeld,
		KeyBuilder:       definitions.MustTemplateKeyBuilder("{"+sdk.ResourceKeyInputKey+"}", []string{sdk.ResourceKeyInputKey}),
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
	normalized := req.cachedNormalized
	if normalized.DefinitionID() == "" {
		normalized = normalizeUseCase(req.useCaseCore, map[string]int{}, req.registryLink)
	}

	return sdk.BindRunRequest(
		normalized,
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
	case errors.Is(err, lockerrors.ErrPreconditionFailed):
		return ErrPreconditionFailed
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
