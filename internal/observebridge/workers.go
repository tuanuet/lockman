package observebridge

import (
	"github.com/tuanuet/lockman/lockkit/workers"
	"github.com/tuanuet/lockman/observe"
)

// PublishWorkerAcquireStarted publishes an acquire-started event for a worker claim.
func (b *Bridge) PublishWorkerAcquireStarted(e observe.Event) {
	e.Kind = observe.EventAcquireStarted
	b.publish(e)
}

// PublishWorkerAcquireSucceeded publishes an acquire-succeeded event for a worker claim.
func (b *Bridge) PublishWorkerAcquireSucceeded(e observe.Event) {
	e.Kind = observe.EventAcquireSucceeded
	b.publish(e)
}

// PublishWorkerAcquireFailed publishes an acquire-failed event with the given error.
func (b *Bridge) PublishWorkerAcquireFailed(e observe.Event, err error) {
	e.Kind = observe.EventAcquireFailed
	e.Error = err
	b.publish(e)
}

// PublishWorkerReleased publishes a released event for a worker claim.
func (b *Bridge) PublishWorkerReleased(e observe.Event) {
	e.Kind = observe.EventReleased
	b.publish(e)
}

// PublishWorkerOverlap publishes an overlap event.
func (b *Bridge) PublishWorkerOverlap(e observe.Event) {
	e.Kind = observe.EventOverlap
	b.publish(e)
}

// PublishWorkerRenewalSucceeded publishes a renewal-succeeded event for a worker claim.
func (b *Bridge) PublishWorkerRenewalSucceeded(e observe.Event) {
	e.Kind = observe.EventRenewalSucceeded
	b.publish(e)
}

// PublishWorkerLeaseLost publishes a lease-lost event for a worker claim.
func (b *Bridge) PublishWorkerLeaseLost(e observe.Event) {
	e.Kind = observe.EventLeaseLost
	b.publish(e)
}

// PublishWorkerShutdownStarted publishes a shutdown-started event for the worker manager.
func (b *Bridge) PublishWorkerShutdownStarted() {
	b.publish(observe.Event{Kind: observe.EventShutdownStarted})
}

// PublishWorkerShutdownCompleted publishes a shutdown-completed event for the worker manager.
func (b *Bridge) PublishWorkerShutdownCompleted() {
	b.publish(observe.Event{Kind: observe.EventShutdownCompleted})
}

var _ workers.Bridge = (*Bridge)(nil)
