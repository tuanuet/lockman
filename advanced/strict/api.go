package strict

import "github.com/tuanuet/lockman"

// DefineRunOn declares a strict fenced run use case on top of an existing lock definition.
func DefineRunOn[T any](name string, def lockman.LockDefinition[T], opts ...lockman.UseCaseOption) lockman.RunUseCase[T] {
	opts = append(opts, lockman.Strict())
	return lockman.DefineRunOn(name, def, opts...)
}
