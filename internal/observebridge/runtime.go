package observebridge

import "github.com/tuanuet/lockman/observe"

// PublishRuntimeAcquireStarted publishes an acquire-started event for a runtime lock.
func (b *Bridge) PublishRuntimeAcquireStarted(re observe.Event) {
	re.Kind = observe.EventAcquireStarted
	b.publish(re)
}

// PublishRuntimeAcquireSucceeded publishes an acquire-succeeded event for a runtime lock.
func (b *Bridge) PublishRuntimeAcquireSucceeded(re observe.Event) {
	re.Kind = observe.EventAcquireSucceeded
	b.publish(re)
}

// PublishRuntimeAcquireFailed publishes an acquire-failed event with the given error.
func (b *Bridge) PublishRuntimeAcquireFailed(re observe.Event, err error) {
	re.Kind = observe.EventAcquireFailed
	re.Error = err
	b.publish(re)
}

// PublishRuntimeReleased publishes a released event for a runtime lock.
func (b *Bridge) PublishRuntimeReleased(re observe.Event) {
	re.Kind = observe.EventReleased
	b.publish(re)
}

// PublishRuntimeContention publishes a contention event.
func (b *Bridge) PublishRuntimeContention(re observe.Event) {
	re.Kind = observe.EventContention
	b.publish(re)
}

// PublishRuntimeOverlapRejected publishes an overlap-rejected event.
func (b *Bridge) PublishRuntimeOverlapRejected(re observe.Event) {
	re.Kind = observe.EventOverlapRejected
	b.publish(re)
}

// PublishRuntimePresenceChecked publishes a presence-checked event.
func (b *Bridge) PublishRuntimePresenceChecked(re observe.Event) {
	re.Kind = observe.EventPresenceChecked
	b.publish(re)
}

// PublishRuntimeRenewalSucceeded publishes a renewal-succeeded event.
func (b *Bridge) PublishRuntimeRenewalSucceeded(re observe.Event) {
	re.Kind = observe.EventRenewalSucceeded
	b.publish(re)
}

// PublishRuntimeRenewalFailed publishes a renewal-failed event with the given error.
func (b *Bridge) PublishRuntimeRenewalFailed(re observe.Event, err error) {
	re.Kind = observe.EventRenewalFailed
	re.Error = err
	b.publish(re)
}

// PublishRuntimeLeaseLost publishes a lease-lost event.
func (b *Bridge) PublishRuntimeLeaseLost(re observe.Event) {
	re.Kind = observe.EventLeaseLost
	b.publish(re)
}

// PublishRuntimeShutdownStarted publishes a shutdown-started event.
func (b *Bridge) PublishRuntimeShutdownStarted() {
	b.publish(observe.Event{Kind: observe.EventShutdownStarted})
}

// PublishRuntimeShutdownCompleted publishes a shutdown-completed event.
func (b *Bridge) PublishRuntimeShutdownCompleted() {
	b.publish(observe.Event{Kind: observe.EventShutdownCompleted})
}
