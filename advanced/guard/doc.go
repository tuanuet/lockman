// Package guard is an explicit advanced path for lower-level context guard
// helpers and policy control.
//
// Prefer the default lockman Run/Claim APIs unless you need direct guard
// composition outside the root client flows. This package is currently a
// namespace/doc entry point; concrete public wrappers are added in the next
// task.
package guard
