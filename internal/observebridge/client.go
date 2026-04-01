package observebridge

import "github.com/tuanuet/lockman/observe"

// PublishClientStarted publishes a client-started event.
func (b *Bridge) PublishClientStarted() {
	b.publish(observe.Event{Kind: observe.EventClientStarted})
}

// PublishClientShutdownStarted publishes a client-shutdown-started event.
func (b *Bridge) PublishClientShutdownStarted() {
	b.publish(observe.Event{Kind: observe.EventShutdownStarted})
}

// PublishClientShutdownCompleted publishes a client-shutdown-completed event.
func (b *Bridge) PublishClientShutdownCompleted() {
	b.publish(observe.Event{Kind: observe.EventShutdownCompleted})
}
