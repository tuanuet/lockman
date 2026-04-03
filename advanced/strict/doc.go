// Package strict is an explicit advanced path for fenced strict-lock behavior.
//
// Prefer the default lockman Run/Claim APIs unless you need direct strict mode
// token semantics. This package keeps the definition-first authoring model and
// exposes strict run attachment on top of the root lockman client path.
package strict
