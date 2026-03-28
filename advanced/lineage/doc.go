// Package lineage is an explicit advanced path for lineage-aware lock flows.
//
// Prefer the default lockman Run/Claim APIs unless you need parent-child lock
// overlap guarantees directly. This package is currently a reserved advanced
// namespace; most applications should stay on the default lockman path.
package lineage
