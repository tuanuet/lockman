package observe

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatcherPublishDoesNotBlockOnSlowSink(t *testing.T) {
	slow := make(chan struct{})
	d := NewDispatcher(
		WithBufferSize(1),
		WithSink(SinkFunc(func(context.Context, Event) error {
			<-slow
			return nil
		})),
	)
	defer func() { _ = d.Shutdown(context.Background()) }()

	done := make(chan struct{})
	go func() {
		d.Publish(Event{Kind: EventAcquireStarted})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("Publish blocked on slow sink")
	}
	close(slow)
}

func TestDispatcherDropsEventsWhenBufferFull(t *testing.T) {
	d := NewDispatcher(
		WithBufferSize(1),
		WithDropPolicy(DropPolicyDropOldest),
	)
	defer func() { _ = d.Shutdown(context.Background()) }()

	d.Publish(Event{Kind: EventAcquireStarted})
	d.Publish(Event{Kind: EventAcquireStarted})
	d.Publish(Event{Kind: EventAcquireStarted})

	if d.DroppedCount() != 2 {
		t.Errorf("DroppedCount() = %d, want 2", d.DroppedCount())
	}
}

func TestDispatcherShutdownDrainsWithDeadline(t *testing.T) {
	d := NewDispatcher(
		WithBufferSize(100),
		WithSink(SinkFunc(func(ctx context.Context, event Event) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		})),
	)

	for i := 0; i < 50; i++ {
		d.Publish(Event{Kind: EventAcquireStarted})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	_ = d.Shutdown(ctx)
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("Shutdown took %v, expected < 100ms (deadline was 20ms)", elapsed)
	}
}

func TestDispatcherContinuesOnSinkFailure(t *testing.T) {
	var sink1Called, sink2Called atomic.Bool

	d := NewDispatcher(
		WithSink(SinkFunc(func(ctx context.Context, event Event) error {
			sink1Called.Store(true)
			return assert.AnError
		})),
		WithSink(SinkFunc(func(ctx context.Context, event Event) error {
			sink2Called.Store(true)
			return nil
		})),
	)
	defer func() { _ = d.Shutdown(context.Background()) }()

	d.Publish(Event{Kind: EventAcquireStarted})

	time.Sleep(50 * time.Millisecond)

	if !sink1Called.Load() {
		t.Error("sink1 was not called")
	}
	if !sink2Called.Load() {
		t.Error("sink2 was not called despite sink1 failing")
	}

	if d.SinkFailureCount() != 1 {
		t.Errorf("SinkFailureCount() = %d, want 1", d.SinkFailureCount())
	}
}

func TestDispatcherPublishReturnsQuickly(t *testing.T) {
	d := NewDispatcher(WithBufferSize(100))
	defer func() { _ = d.Shutdown(context.Background()) }()

	start := time.Now()
	d.Publish(Event{Kind: EventAcquireStarted})
	elapsed := time.Since(start)

	if elapsed > 10*time.Millisecond {
		t.Errorf("Publish took %v, want < 10ms", elapsed)
	}
}

func TestDispatcherWithMultipleExportersContinuesOnFailure(t *testing.T) {
	var exporter1Called, exporter2Called atomic.Bool

	d := NewDispatcher(
		WithBufferSize(100),
		WithExporter(ExporterFunc(func(ctx context.Context, event Event) error {
			exporter1Called.Store(true)
			return assert.AnError
		})),
		WithExporter(ExporterFunc(func(ctx context.Context, event Event) error {
			exporter2Called.Store(true)
			return nil
		})),
	)
	defer func() { _ = d.Shutdown(context.Background()) }()

	d.Publish(Event{Kind: EventAcquireStarted})

	time.Sleep(50 * time.Millisecond)

	if !exporter1Called.Load() {
		t.Error("exporter1 was not called")
	}
	if !exporter2Called.Load() {
		t.Error("exporter2 was not called despite exporter1 failing")
	}

	if d.ExporterFailureCount() != 1 {
		t.Errorf("ExporterFailureCount() = %d, want 1", d.ExporterFailureCount())
	}
}

var assert = struct {
	AnError error
}{
	AnError: assertAnError{},
}

type assertAnError struct{}

func (assertAnError) Error() string { return "assert.AnError" }
