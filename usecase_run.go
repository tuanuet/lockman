package lockman

import (
	"fmt"
	"strings"
)

// RunUseCase defines a typed synchronous use case.
type RunUseCase[T any] struct {
	core    *useCaseCore
	binding Binding[T]
}

// DefineRun declares a typed run use case.
func DefineRun[T any](name string, binding Binding[T], opts ...UseCaseOption) RunUseCase[T] {
	if strings.TrimSpace(name) == "" || binding.build == nil {
		return RunUseCase[T]{
			core:    newUseCaseCore(name, useCaseKindRun, opts...),
			binding: binding,
		}
	}
	def := DefineLock(name, binding)
	return DefineRunOn(name, def, opts...)
}

// DefineRunOn declares a typed run use case on top of a shared lock definition.
func DefineRunOn[T any](name string, def LockDefinition[T], opts ...UseCaseOption) RunUseCase[T] {
	return RunUseCase[T]{
		core:    newUseCaseCoreWithDefinition(name, useCaseKindRun, def.ref, opts...),
		binding: def.binding,
	}
}

// DefinitionID returns the use case definition name.
func (u RunUseCase[T]) DefinitionID() string {
	if u.core == nil {
		return ""
	}
	return u.core.name
}

// With binds typed input into an opaque run request.
func (u RunUseCase[T]) With(input T, opts ...CallOption) (RunRequest, error) {
	if u.core == nil {
		return RunRequest{}, fmt.Errorf("lockman: run use case is not defined")
	}
	if len(u.core.config.composite) == 0 && u.binding.build == nil {
		return RunRequest{}, fmt.Errorf("lockman: run use case binding is required")
	}

	call := applyCallOptions(opts...)
	if call.ownerIDSet && call.ownerID == "" {
		return RunRequest{}, fmt.Errorf("lockman: owner override is required: %w", ErrIdentityRequired)
	}

	req := RunRequest{
		useCaseName: u.core.name,
		ownerID:     call.ownerID,
		useCaseCore: u.core,
	}
	if len(u.core.config.composite) > 0 {
		memberInputs, err := buildCompositeMemberInputs(any(input), u.core.config.composite)
		if err != nil {
			return RunRequest{}, fmt.Errorf("lockman: bind composite run request: %w", err)
		}
		req.compositeMemberInputs = memberInputs
	} else {
		resourceKey, err := u.binding.build(input)
		if err != nil {
			return RunRequest{}, fmt.Errorf("lockman: bind run request: %w", err)
		}
		req.resourceKey = resourceKey
	}
	if u.core.registry != nil {
		req.registryLink = u.core.registry.link
		req.boundToRegistry = true
	}

	return req, nil
}

func buildCompositeMemberInputs(input any, members []compositeMemberConfig) ([]map[string]string, error) {
	memberInputs := make([]map[string]string, 0, len(members))
	for _, member := range members {
		if member.build == nil {
			return nil, errBindingFunctionRequired
		}
		if member.name == "" {
			return nil, fmt.Errorf("lockman: composite member name is required")
		}
		bound, err := member.build(input)
		if err != nil {
			return nil, err
		}
		memberInputs = append(memberInputs, bound)
	}
	return memberInputs, nil
}

func (u RunUseCase[T]) sdkUseCase() *useCaseCore {
	return u.core
}
