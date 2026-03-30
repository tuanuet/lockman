// Package guard is an explicit advanced path for guard-oriented helpers and
// policy control.
//
// The stable, low-level adapter-facing contract is the top-level lockman/guard
// package (Context/Outcome only). This advanced namespace is for higher-level
// composition beyond the default client flows.
//
// Prefer the default lockman Run/Claim APIs unless you need direct guard
// composition outside the root client flows. This package is currently a
// reserved advanced namespace; most applications should stay on the default
// lockman path.
package guard
