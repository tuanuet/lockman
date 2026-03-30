package strict

import "github.com/tuanuet/lockman"

// DefineRun declares a strict fenced run use case on top of the root lockman client path.
func DefineRun[T any](name string, binding lockman.Binding[T], opts ...lockman.UseCaseOption) lockman.RunUseCase[T] {
	opts = append(opts, lockman.Strict())
	return lockman.DefineRun(name, binding, opts...)
}
