// Package strict is an explicit advanced path for fenced strict-lock behavior.
//
// Prefer the default lockman Run/Claim APIs unless you need direct strict mode
// token semantics.
//
// Deprecated: Use root lockman.DefineLock with lockman.StrictDef() to create
// a strict lock definition, then lockman.DefineRunOn to attach a run use case:
//
//	strictDef := lockman.DefineLock(
//		"order.strict-write",
//		lockman.BindResourceID("order", func(v string) string { return v }),
//		lockman.StrictDef(),
//	)
//	approve := lockman.DefineRunOn("order.strict-approve", strictDef)
package strict
