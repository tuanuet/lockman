// Package redis exposes the user-facing Redis idempotency store constructor.
//
// Most users should wire this with lockman.WithIdempotency(...) and keep using
// the default lockman Claim flow. Passing an empty keyPrefix uses lockman's
// default Redis idempotency namespace.
package redis
