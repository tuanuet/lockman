package otel_test

import (
	"context"
	"testing"
	"time"

	"github.com/tuanuet/lockman/observe"
	"github.com/tuanuet/lockman/observe/otel"
)

func TestOTelSinkSatisfiesSinkInterface(t *testing.T) {
	var _ observe.Sink = otel.NewOTelSink(otel.OTelConfig{})
}

func TestOTelSinkConsumeNeverReturnsError(t *testing.T) {
	sink := otel.NewOTelSink(otel.OTelConfig{})
	err := sink.Consume(context.Background(), observe.Event{
		Kind: observe.EventAcquireSucceeded,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestOTelSinkWithNilProvidersIsNoop(t *testing.T) {
	sink := otel.NewOTelSink(otel.OTelConfig{
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

func TestOTelSinkConsumeWithOnlyTracerProvider(t *testing.T) {
	sink := otel.NewOTelSink(otel.OTelConfig{
		TracerProvider: nil,
		MeterProvider:  nil,
	})
	err := sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "test-def",
		RequestID:    "req-1",
		OwnerID:      "owner-1",
		ResourceID:   "res-1",
		Wait:         50 * time.Millisecond,
		Success:      true,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestOTelSinkConsumeWithEventError(t *testing.T) {
	sink := otel.NewOTelSink(otel.OTelConfig{})
	testErr := context.DeadlineExceeded
	err := sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireFailed,
		DefinitionID: "test-def",
		Error:        testErr,
		Success:      false,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestOTelSinkConsumeAllEventKinds(t *testing.T) {
	sink := otel.NewOTelSink(otel.OTelConfig{})
	ctx := context.Background()

	kinds := []observe.EventKind{
		observe.EventAcquireStarted,
		observe.EventAcquireSucceeded,
		observe.EventAcquireFailed,
		observe.EventReleased,
		observe.EventContention,
		observe.EventOverlap,
		observe.EventLeaseLost,
		observe.EventRenewalSucceeded,
		observe.EventRenewalFailed,
		observe.EventShutdownStarted,
		observe.EventShutdownCompleted,
		observe.EventClientStarted,
		observe.EventOverlapRejected,
		observe.EventPresenceChecked,
	}

	for _, kind := range kinds {
		event := observe.Event{
			Kind:         kind,
			DefinitionID: "def-1",
			RequestID:    "req-1",
			OwnerID:      "owner-1",
			ResourceID:   "res-1",
			Wait:         100 * time.Millisecond,
			Held:         2 * time.Second,
			Contention:   3,
			Success:      true,
			Timestamp:    time.Now(),
			Error:        nil,
		}

		err := sink.Consume(ctx, event)
		if err != nil {
			t.Errorf("Consume(%s) returned error: %v", kind, err)
		}
	}
}

func TestOTelSinkConsumeWithContention(t *testing.T) {
	sink := otel.NewOTelSink(otel.OTelConfig{})
	err := sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventContention,
		DefinitionID: "def-1",
		Contention:   5,
		Success:      false,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestOTelSinkConsumeWithHoldDuration(t *testing.T) {
	sink := otel.NewOTelSink(otel.OTelConfig{})
	err := sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventReleased,
		DefinitionID: "def-1",
		Held:         5 * time.Second,
		Success:      true,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestOTelSinkConsumeWithZeroDurations(t *testing.T) {
	sink := otel.NewOTelSink(otel.OTelConfig{})
	err := sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "def-1",
		Wait:         0,
		Held:         0,
		Success:      true,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
