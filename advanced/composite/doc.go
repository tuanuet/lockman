// Package composite is an explicit advanced path for composing multiple lock
// definitions in one operation.
//
// Prefer the default lockman Run/Claim APIs unless you need custom multi-lock
// orchestration behavior. This package exposes advanced composite run use-case
// authoring on top of the root lockman client path.
package composite
