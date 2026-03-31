package observebridge

import "github.com/tuanuet/lockman/observe"

// PublishWorkerAcquireStarted publishes an acquire-started event for a worker claim.
func (b *Bridge) PublishWorkerAcquireStarted(we WorkerEvent) {
	b.publish(buildWorkerEvent(observe.EventAcquireStarted, we))
}

// PublishWorkerAcquireSucceeded publishes an acquire-succeeded event for a worker claim.
func (b *Bridge) PublishWorkerAcquireSucceeded(we WorkerEvent) {
	b.publish(buildWorkerEvent(observe.EventAcquireSucceeded, we))
}

// PublishWorkerAcquireFailed publishes an acquire-failed event with the given error.
func (b *Bridge) PublishWorkerAcquireFailed(we WorkerEvent, err error) {
	event := buildWorkerEvent(observe.EventAcquireFailed, we)
	event.Error = err
	b.publish(event)
}

// PublishWorkerReleased publishes a released event for a worker claim.
func (b *Bridge) PublishWorkerReleased(we WorkerEvent) {
	b.publish(buildWorkerEvent(observe.EventReleased, we))
}

// PublishWorkerOverlap publishes an overlap event.
func (b *Bridge) PublishWorkerOverlap(we WorkerEvent) {
	b.publish(buildWorkerEvent(observe.EventOverlap, we))
}
