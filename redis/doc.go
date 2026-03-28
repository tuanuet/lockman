// Package redis exposes the user-facing Redis backend constructor for lockman.
//
// Most users should wire this with lockman.WithBackend(...) and stay on the
// default lockman Run/Claim APIs.
package redis
