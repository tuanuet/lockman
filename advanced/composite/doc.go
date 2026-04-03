// Package composite is an explicit advanced path for composing multiple lock
// definitions in one operation.
//
// Prefer the default lockman Run/Claim APIs unless you need custom multi-lock
// orchestration behavior. This package keeps child definitions explicit and
// exposes advanced composite definition-plus-attach authoring on top of the
// root lockman client path.
package composite
