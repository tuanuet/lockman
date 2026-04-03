package lockman

import (
	"fmt"
)

// HoldHandle carries an opaque hold token.
type HoldHandle struct {
	token string
}

// Token returns the opaque hold token.
func (h HoldHandle) Token() string {
	return h.token
}

// HoldUseCase defines a typed hold use case.
type HoldUseCase[T any] struct {
	core    *useCaseCore
	binding Binding[T]
}

// DefineHoldOn declares a typed hold use case on top of a shared lock definition.
func DefineHoldOn[T any](name string, def LockDefinition[T], opts ...UseCaseOption) HoldUseCase[T] {
	if def.ref.config.strict {
		panic("lockman: hold use case cannot use strict definition")
	}
	return HoldUseCase[T]{
		core:    newUseCaseCoreWithDefinition(name, useCaseKindHold, def.ref, opts...),
		binding: def.binding,
	}
}

// With binds typed input into an opaque hold request.
func (u HoldUseCase[T]) With(input T, opts ...CallOption) (HoldRequest, error) {
	if u.core == nil {
		return HoldRequest{}, fmt.Errorf("lockman: hold use case is not defined")
	}
	if u.binding.build == nil {
		return HoldRequest{}, fmt.Errorf("lockman: hold use case binding is required")
	}

	call := applyCallOptions(opts...)
	if call.ownerIDSet && call.ownerID == "" {
		return HoldRequest{}, fmt.Errorf("lockman: owner override is required: %w", ErrIdentityRequired)
	}

	resourceKey, err := u.binding.build(input)
	if err != nil {
		return HoldRequest{}, fmt.Errorf("lockman: bind hold request: %w", err)
	}

	req := HoldRequest{
		useCaseName: u.core.name,
		resourceKey: resourceKey,
		ownerID:     call.ownerID,
		useCaseCore: u.core,
	}
	if u.core.registry != nil {
		req.registryLink = u.core.registry.link
		req.boundToRegistry = true
	}

	return req, nil
}

// ForfeitWith binds a raw hold token into an opaque forfeit request.
func (u HoldUseCase[T]) ForfeitWith(token string) ForfeitRequest {
	req := ForfeitRequest{
		token:       token,
		useCaseCore: u.core,
	}
	if u.core != nil {
		req.useCaseName = u.core.name
	}
	if u.core != nil && u.core.registry != nil {
		req.registryLink = u.core.registry.link
		req.boundToRegistry = true
	}
	return req
}

// DefinitionID returns the use case definition name.
func (u HoldUseCase[T]) DefinitionID() string {
	if u.core == nil {
		return ""
	}
	return u.core.name
}

func (u HoldUseCase[T]) sdkUseCase() *useCaseCore {
	return u.core
}
