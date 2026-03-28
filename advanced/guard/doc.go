// Package guard is an explicit advanced path for lower-level context guard
// helpers and policy control.
//
// Prefer the default lockman Run/Claim APIs unless you need direct guard
// composition outside the root client flows. This package is currently a
// reserved advanced namespace; most applications should stay on the default
// lockman path.
package guard
