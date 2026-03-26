package registry

import (
	"errors"
	"fmt"
	"strings"

	"lockman/lockkit/definitions"
)

type definitionValidation func(definitions.LockDefinition) error

var definitionValidations = []definitionValidation{
	requireDefinitionID,
	requireDefinitionResource,
	requireKeyBuilder,
	requireStrictFencing,
	requireStrictFailClosed,
	requireStrictAsyncIdempotency,
}

var (
	errDefinitionIDRequired           = errors.New("definition id must not be empty")
	errDefinitionResourceRequired     = errors.New("definition resource must not be empty")
	errDefinitionKeyBuilderRequired   = errors.New("definition must provide a key builder")
	errStrictModeRequiresFencing      = errors.New("strict definitions require fencing")
	errStrictModeRequiresFailClosed   = errors.New("strict definitions require explicit fail_closed backend policy")
	errStrictAsyncRequiresIdempotency = errors.New("strict async and strict both definitions require idempotency")
	errChildOverlapPolicyUnsupported  = errors.New("child definitions only support overlap policy reject in phase 2")

	errCompositeDefinitionIDRequired        = errors.New("composite definition id must not be empty")
	errCompositeMembersRequired             = errors.New("composite definition must include at least one member")
	errCompositeOrderingPolicyUnsupported   = errors.New("composite definition must use canonical ordering in phase 2")
	errCompositeAcquirePolicyUnsupported    = errors.New("composite definition must use all_or_nothing acquire policy in phase 2")
	errCompositeEscalationPolicyUnsupported = errors.New("composite definition must use reject escalation policy in phase 2")
	errCompositeModeResolutionUnsupported   = errors.New("composite definition must use homogeneous mode resolution in phase 2")
	errCompositeMaxMemberCountInvalid       = errors.New("composite definition max member count must be positive")
	errCompositeMaxMemberCountExceeded      = errors.New("composite definition exceeds max member count")
	errCompositeStrictMemberUnsupported     = errors.New("composite definitions cannot include strict members in phase 2")
	errCompositeMixedModesUnsupported       = errors.New("composite definition members must resolve to a homogeneous mode")
)

// ValidateDefinition applies the configured validators for a single definition.
func ValidateDefinition(def definitions.LockDefinition) error {
	for _, validate := range definitionValidations {
		if err := validate(def); err != nil {
			return err
		}
	}
	return nil
}

func requireDefinitionID(def definitions.LockDefinition) error {
	if strings.TrimSpace(def.ID) == "" {
		return errDefinitionIDRequired
	}
	return nil
}

func requireDefinitionResource(def definitions.LockDefinition) error {
	if strings.TrimSpace(def.Resource) == "" {
		return errDefinitionResourceRequired
	}
	return nil
}

func requireKeyBuilder(def definitions.LockDefinition) error {
	if def.KeyBuilder == nil {
		return errDefinitionKeyBuilderRequired
	}
	return nil
}

func requireStrictFencing(def definitions.LockDefinition) error {
	if def.Mode == definitions.ModeStrict && !def.FencingRequired {
		return errStrictModeRequiresFencing
	}
	return nil
}

func requireStrictFailClosed(def definitions.LockDefinition) error {
	if def.Mode == definitions.ModeStrict && def.BackendFailurePolicy != definitions.BackendFailClosed {
		return errStrictModeRequiresFailClosed
	}
	return nil
}

func requireStrictAsyncIdempotency(def definitions.LockDefinition) error {
	if def.Mode != definitions.ModeStrict {
		return nil
	}
	if def.ExecutionKind != definitions.ExecutionAsync && def.ExecutionKind != definitions.ExecutionBoth {
		return nil
	}
	if !def.IdempotencyRequired {
		return errStrictAsyncRequiresIdempotency
	}
	return nil
}

// ValidateDefinitionAgainstRegistry applies validations that need access to registry definitions.
func ValidateDefinitionAgainstRegistry(def definitions.LockDefinition, definitionsByID map[string]definitions.LockDefinition) error {
	if def.Kind != definitions.KindChild {
		return nil
	}
	parentID := strings.TrimSpace(def.ParentRef)
	if parentID == "" {
		return fmt.Errorf("child definition references unknown parent %q", def.ParentRef)
	}
	if _, exists := definitionsByID[parentID]; !exists {
		return fmt.Errorf("child definition references unknown parent %q", def.ParentRef)
	}
	if def.OverlapPolicy != definitions.OverlapReject {
		return errChildOverlapPolicyUnsupported
	}
	return nil
}

// ValidateCompositeDefinition validates a composite definition against registered members.
func ValidateCompositeDefinition(def definitions.CompositeDefinition, definitionsByID map[string]definitions.LockDefinition) error {
	if err := requireCompositeDefinitionID(def); err != nil {
		return err
	}
	if len(def.Members) == 0 {
		return errCompositeMembersRequired
	}
	if def.OrderingPolicy != definitions.OrderingCanonical {
		return errCompositeOrderingPolicyUnsupported
	}
	if def.AcquirePolicy != definitions.AcquireAllOrNothing {
		return errCompositeAcquirePolicyUnsupported
	}
	if def.EscalationPolicy != definitions.EscalationReject {
		return errCompositeEscalationPolicyUnsupported
	}
	if def.ModeResolution != definitions.ModeResolutionHomogeneous {
		return errCompositeModeResolutionUnsupported
	}
	if def.MaxMemberCount <= 0 {
		return errCompositeMaxMemberCountInvalid
	}
	if len(def.Members) > def.MaxMemberCount {
		return errCompositeMaxMemberCountExceeded
	}

	var expectedMode definitions.LockMode
	for index, memberID := range def.Members {
		member, exists := definitionsByID[memberID]
		if !exists {
			return fmt.Errorf("composite definition references unknown member %q", memberID)
		}
		if member.Mode == definitions.ModeStrict {
			return errCompositeStrictMemberUnsupported
		}
		if index == 0 {
			expectedMode = member.Mode
			continue
		}
		if member.Mode != expectedMode {
			return errCompositeMixedModesUnsupported
		}
	}

	return nil
}

func requireCompositeDefinitionID(def definitions.CompositeDefinition) error {
	if strings.TrimSpace(def.ID) == "" {
		return errCompositeDefinitionIDRequired
	}
	return nil
}
