package registry

import (
	"errors"
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
}

var (
	errDefinitionIDRequired         = errors.New("definition id must not be empty")
	errDefinitionResourceRequired   = errors.New("definition resource must not be empty")
	errDefinitionKeyBuilderRequired = errors.New("definition must provide a key builder")
	errStrictModeRequiresFencing    = errors.New("strict definitions require fencing")
	errStrictModeRequiresFailClosed = errors.New("strict definitions require explicit fail_closed backend policy")
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
