// Package drivers is a temporary compatibility layer.
//
// The stable adapter-facing contracts live in the top-level lockman/backend package.
// New code should depend on lockman/backend directly.
package drivers

import "lockman/backend"

// Sentinel errors are owned by the backend contract package.
var (
	ErrInvalidRequest     = backend.ErrInvalidRequest
	ErrLeaseAlreadyHeld   = backend.ErrLeaseAlreadyHeld
	ErrLeaseNotFound      = backend.ErrLeaseNotFound
	ErrLeaseExpired       = backend.ErrLeaseExpired
	ErrLeaseOwnerMismatch = backend.ErrLeaseOwnerMismatch
)

type (
	Driver                = backend.Driver
	StrictDriver          = backend.StrictDriver
	LineageDriver         = backend.LineageDriver
	AcquireRequest        = backend.AcquireRequest
	LeaseRecord           = backend.LeaseRecord
	FencedLeaseRecord     = backend.FencedLeaseRecord
	PresenceRequest       = backend.PresenceRequest
	PresenceRecord        = backend.PresenceRecord
	StrictAcquireRequest  = backend.StrictAcquireRequest
	AncestorKey           = backend.AncestorKey
	LineageLeaseMeta      = backend.LineageLeaseMeta
	LineageAcquireRequest = backend.LineageAcquireRequest
	LockKind              = backend.LockKind
)

const (
	KindParent LockKind = backend.KindParent
	KindChild  LockKind = backend.KindChild
)
