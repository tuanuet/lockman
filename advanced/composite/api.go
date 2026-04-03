package composite

import "github.com/tuanuet/lockman"

// Definition describes one composite lock boundary over typed child definitions.
type Definition[T any] struct {
	members []lockman.CompositeMember[T]
}

// DefineLock declares one composite lock boundary from child lock definitions.
func DefineLock[T any](name string, defs ...lockman.LockDefinition[T]) Definition[T] {
	members := make([]lockman.CompositeMember[T], 0, len(defs))
	for _, def := range defs {
		members = append(members, lockman.Member[T](def.DefinitionID(), def, func(in T) T { return in }))
	}
	return Definition[T]{
		members: members,
	}
}

// AttachRun declares a composite synchronous run use case on top of the root lockman client path.
func AttachRun[T any](name string, def Definition[T], opts ...lockman.UseCaseOption) lockman.RunUseCase[T] {
	return lockman.DefineCompositeRunWithOptions(name, opts, def.members...)
}
