package observebridge

import "github.com/tuanuet/lockman/observe"

// PublishRuntimeAcquireStarted publishes an acquire-started event for a runtime lock.
func (b *Bridge) PublishRuntimeAcquireStarted(re RuntimeEvent) {
	b.publish(buildRuntimeEvent(observe.EventAcquireStarted, re))
}

// PublishRuntimeAcquireSucceeded publishes an acquire-succeeded event for a runtime lock.
func (b *Bridge) PublishRuntimeAcquireSucceeded(re RuntimeEvent) {
	b.publish(buildRuntimeEvent(observe.EventAcquireSucceeded, re))
}

// PublishRuntimeAcquireFailed publishes an acquire-failed event with the given error.
func (b *Bridge) PublishRuntimeAcquireFailed(re RuntimeEvent, err error) {
	event := buildRuntimeEvent(observe.EventAcquireFailed, re)
	event.Error = err
	b.publish(event)
}

// PublishRuntimeReleased publishes a released event for a runtime lock.
func (b *Bridge) PublishRuntimeReleased(re RuntimeEvent) {
	b.publish(buildRuntimeEvent(observe.EventReleased, re))
}

// PublishRuntimeRenewalSucceeded publishes a renewal-succeeded event.
func (b *Bridge) PublishRuntimeRenewalSucceeded(re RuntimeEvent) {
	b.publish(buildRuntimeEvent(observe.EventRenewalSucceeded, re))
}

// PublishRuntimeRenewalFailed publishes a renewal-failed event with the given error.
func (b *Bridge) PublishRuntimeRenewalFailed(re RuntimeEvent, err error) {
	event := buildRuntimeEvent(observe.EventRenewalFailed, re)
	event.Error = err
	b.publish(event)
}

// PublishRuntimeLeaseLost publishes a lease-lost event.
func (b *Bridge) PublishRuntimeLeaseLost(re RuntimeEvent) {
	b.publish(buildRuntimeEvent(observe.EventLeaseLost, re))
}

// PublishRuntimeContention publishes a contention event.
func (b *Bridge) PublishRuntimeContention(re RuntimeEvent) {
	b.publish(buildRuntimeEvent(observe.EventContention, re))
}

// PublishShutdownStarted publishes a shutdown-started event.
func (b *Bridge) PublishShutdownStarted() {
	b.publish(observe.Event{Kind: observe.EventShutdownStarted})
}

// PublishShutdownCompleted publishes a shutdown-completed event.
func (b *Bridge) PublishShutdownCompleted() {
	b.publish(observe.Event{Kind: observe.EventShutdownCompleted})
}
