package observebridge

import (
	"time"

	"github.com/tuanuet/lockman/observe"
)

// RuntimeEvent carries the fields for a runtime lifecycle event.
type RuntimeEvent struct {
	DefinitionID string
	ResourceID   string
	OwnerID      string
	RequestID    string
	Wait         time.Duration
	Held         time.Duration
	Contention   int
}

// WorkerEvent carries the fields for a worker lifecycle event.
type WorkerEvent struct {
	DefinitionID string
	ResourceID   string
	OwnerID      string
	RequestID    string
	Wait         time.Duration
	Held         time.Duration
	Contention   int
}

// buildEvent converts common fields into an observe.Event.
func buildEvent(kind observe.EventKind, defID, resourceID, ownerID, requestID string) observe.Event {
	return observe.Event{
		Kind:         kind,
		DefinitionID: defID,
		ResourceID:   resourceID,
		OwnerID:      ownerID,
		RequestID:    requestID,
	}
}

// buildRuntimeEvent converts a RuntimeEvent into an observe.Event.
func buildRuntimeEvent(kind observe.EventKind, re RuntimeEvent) observe.Event {
	return observe.Event{
		Kind:         kind,
		DefinitionID: re.DefinitionID,
		ResourceID:   re.ResourceID,
		OwnerID:      re.OwnerID,
		RequestID:    re.RequestID,
		Wait:         re.Wait,
		Held:         re.Held,
		Contention:   re.Contention,
	}
}

// buildWorkerEvent converts a WorkerEvent into an observe.Event.
func buildWorkerEvent(kind observe.EventKind, we WorkerEvent) observe.Event {
	return observe.Event{
		Kind:         kind,
		DefinitionID: we.DefinitionID,
		ResourceID:   we.ResourceID,
		OwnerID:      we.OwnerID,
		RequestID:    we.RequestID,
		Wait:         we.Wait,
		Held:         we.Held,
		Contention:   we.Contention,
	}
}
