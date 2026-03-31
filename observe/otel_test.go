package observe_test

import (
	"context"
	"testing"

	"github.com/tuanuet/lockman/observe"
)

func TestOTelSinkSatisfiesSinkInterface(t *testing.T) {
	var _ observe.Sink = observe.NewOTelSink(observe.OTelConfig{})
}

func TestOTelSinkConsumeNeverReturnsError(t *testing.T) {
	sink := observe.NewOTelSink(observe.OTelConfig{})
	err := sink.Consume(context.Background(), observe.Event{
		Kind: observe.EventAcquireSucceeded,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestOTelSinkWithNilProvidersIsNoop(t *testing.T) {
	sink := observe.NewOTelSink(observe.OTelConfig{
		TracerProvider: nil,
		MeterProvider:  nil,
	})
	for _, kind := range []observe.EventKind{
		observe.EventAcquireStarted,
		observe.EventAcquireSucceeded,
		observe.EventAcquireFailed,
		observe.EventReleased,
		observe.EventLeaseLost,
		observe.EventRenewalSucceeded,
		observe.EventRenewalFailed,
		observe.EventShutdownStarted,
		observe.EventShutdownCompleted,
		observe.EventContention,
		observe.EventOverlapRejected,
	} {
		err := sink.Consume(context.Background(), observe.Event{Kind: kind})
		if err != nil {
			t.Fatalf("expected nil error for kind %s, got %v", kind, err)
		}
	}
}
