package composite

import "github.com/tuanuet/lockman"

// Member describes one typed composite member in declaration order.
type Member[T any] struct {
	member lockman.CompositeMember[T]
}

// DefineMember declares one composite member using the root lockman binding model.
func DefineMember[T any](name string, binding lockman.Binding[T]) Member[T] {
	return Member[T]{
		member: lockman.DefineCompositeMember(name, binding),
	}
}

// DefineRun declares a composite synchronous run use case on top of the root lockman client path.
func DefineRun[T any](name string, members ...Member[T]) lockman.RunUseCase[T] {
	return DefineRunWithOptions(name, nil, members...)
}

// DefineRunWithOptions declares a composite synchronous run use case with root run options.
func DefineRunWithOptions[T any](name string, opts []lockman.UseCaseOption, members ...Member[T]) lockman.RunUseCase[T] {
	compositeMembers := make([]lockman.CompositeMember[T], 0, len(members))
	for _, member := range members {
		compositeMembers = append(compositeMembers, member.member)
	}
	opts = append(append([]lockman.UseCaseOption(nil), opts...), lockman.Composite(compositeMembers...))
	return lockman.DefineRun(name, lockman.Binding[T]{}, opts...)
}
