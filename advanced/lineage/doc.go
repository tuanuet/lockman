// Package lineage is an explicit advanced path for lineage-aware lock flows.
//
// Prefer the default lockman Run/Claim APIs unless you need parent-child lock
// overlap guarantees directly. This package is currently a namespace/doc entry
// point; concrete public wrappers are added in the next task.
package lineage
