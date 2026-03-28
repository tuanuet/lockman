package lockman

import (
	"fmt"
	"strings"
)

// ClaimUseCase defines a typed asynchronous claim use case.
type ClaimUseCase[T any] struct {
	core    *useCaseCore
	binding Binding[T]
}

// DefineClaim declares a typed claim use case.
func DefineClaim[T any](name string, binding Binding[T], opts ...UseCaseOption) ClaimUseCase[T] {
	return ClaimUseCase[T]{
		core:    newUseCaseCore(name, useCaseKindClaim, opts...),
		binding: binding,
	}
}

// With binds typed input and delivery metadata into an opaque claim request.
func (u ClaimUseCase[T]) With(input T, delivery Delivery, opts ...CallOption) (ClaimRequest, error) {
	if u.core == nil {
		return ClaimRequest{}, fmt.Errorf("lockman: claim use case is not defined")
	}
	if u.binding.build == nil {
		return ClaimRequest{}, fmt.Errorf("lockman: claim use case binding is required")
	}
	if err := validateDelivery(delivery); err != nil {
		return ClaimRequest{}, err
	}
	resourceKey, err := u.binding.build(input)
	if err != nil {
		return ClaimRequest{}, fmt.Errorf("lockman: bind claim request: %w", err)
	}

	call := applyCallOptions(opts...)
	return ClaimRequest{
		useCaseName: u.core.name,
		resourceKey: resourceKey,
		ownerID:     call.ownerID,
		delivery:    delivery,
		useCaseCore: u.core,
	}, nil
}

func (u ClaimUseCase[T]) sdkUseCase() *useCaseCore {
	return u.core
}

func validateDelivery(delivery Delivery) error {
	if strings.TrimSpace(delivery.MessageID) == "" {
		return fmt.Errorf("lockman: delivery message id is required")
	}
	if strings.TrimSpace(delivery.ConsumerGroup) == "" {
		return fmt.Errorf("lockman: delivery consumer group is required")
	}
	if delivery.Attempt <= 0 {
		return fmt.Errorf("lockman: delivery attempt must be positive")
	}
	return nil
}
