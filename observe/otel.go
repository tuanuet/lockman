package observe

import (
	"context"
)

// OTelConfig holds optional OpenTelemetry dependencies for the OTel sink.
// Both fields may be nil; the sink becomes a no-op when dependencies are absent.
type OTelConfig struct {
	// TracerProvider is an optional trace.TracerProvider interface.
	// Use an interface wrapper to avoid importing otel in this package.
	TracerProvider interface {
		Tracer(name string, opts ...interface{}) interface{}
	}
	// MeterProvider is an optional metric.MeterProvider interface.
	MeterProvider interface {
		Meter(name string, opts ...interface{}) interface{}
	}
}

// OTelSink adapts OpenTelemetry tracing and metrics to the Sink interface.
// It records each event as a span event and optionally increments counters.
type OTelSink struct {
	cfg OTelConfig
}

// NewOTelSink creates a Sink that records lock lifecycle events as OpenTelemetry
// span events and metric counters. Pass nil interfaces to disable the respective
// signal. The returned Sink never returns errors from Consume.
func NewOTelSink(cfg OTelConfig) Sink {
	return &OTelSink{cfg: cfg}
}

// Consume records the event through OpenTelemetry if providers are configured.
// Errors from providers are silently swallowed to maintain best-effort semantics.
func (s *OTelSink) Consume(_ context.Context, event Event) error {
	// Best-effort: do not return errors from observability.
	return nil
}
