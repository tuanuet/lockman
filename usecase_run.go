package lockman

import "fmt"

// RunUseCase defines a typed synchronous use case.
type RunUseCase[T any] struct {
	core    *useCaseCore
	binding Binding[T]
}

// DefineRun declares a typed run use case.
func DefineRun[T any](name string, binding Binding[T], opts ...UseCaseOption) RunUseCase[T] {
	return RunUseCase[T]{
		core:    newUseCaseCore(name, useCaseKindRun, opts...),
		binding: binding,
	}
}

// With binds typed input into an opaque run request.
func (u RunUseCase[T]) With(input T, opts ...CallOption) (RunRequest, error) {
	if u.core == nil {
		return RunRequest{}, fmt.Errorf("lockman: run use case is not defined")
	}
	if u.binding.build == nil {
		return RunRequest{}, fmt.Errorf("lockman: run use case binding is required")
	}
	resourceKey, err := u.binding.build(input)
	if err != nil {
		return RunRequest{}, fmt.Errorf("lockman: bind run request: %w", err)
	}

	call := applyCallOptions(opts...)
	return RunRequest{
		useCaseName: u.core.name,
		resourceKey: resourceKey,
		ownerID:     call.ownerID,
		useCaseCore: u.core,
	}, nil
}

func (u RunUseCase[T]) sdkUseCase() *useCaseCore {
	return u.core
}
