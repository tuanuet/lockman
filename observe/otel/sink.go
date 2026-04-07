package otel

import (
	"context"

	"github.com/tuanuet/lockman/observe"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// OTelConfig holds OpenTelemetry providers for the OTel sink.
// Both fields may be nil; the sink becomes a no-op when both are absent.
//
// Datadog integration: configure providers using the Datadog OTel exporter
// or dd-trace-go's OTel bridge, then pass them here. No Datadog-specific
// code is required — standard OTel attributes map to Datadog tags automatically.
//
//	tp := oteltrace.NewTracerProvider(trace.WithServiceName("my-service"))
//	mp := otelmeter.NewMeterProvider(...)
//	sink := otel.NewOTelSink(otel.OTelConfig{
//	    TracerProvider: tp,
//	    MeterProvider:  mp,
//	})
type OTelConfig struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
}

// OTelSink adapts OpenTelemetry tracing and metrics to the Sink interface.
type OTelSink struct {
	tracer trace.Tracer
	meter  metric.Meter

	acquireTotal    metric.Int64Counter
	acquireDuration metric.Float64Histogram
	contentionCount metric.Int64Counter
	holdDuration    metric.Float64Histogram
}

// NewOTelSink creates a Sink that records lock lifecycle events as OpenTelemetry
// spans and metrics. Pass nil providers to disable the respective signal.
// The returned sink never returns errors from Consume.
func NewOTelSink(cfg OTelConfig) observe.Sink {
	s := &OTelSink{}

	if cfg.TracerProvider != nil {
		s.tracer = cfg.TracerProvider.Tracer("github.com/tuanuet/lockman")
	}

	if cfg.MeterProvider != nil {
		s.meter = cfg.MeterProvider.Meter("github.com/tuanuet/lockman")
		s.initMetrics()
	}

	return s
}

func (s *OTelSink) initMetrics() {
	s.acquireTotal, _ = s.meter.Int64Counter(
		"lockman.acquire.total",
		metric.WithDescription("Total number of lock acquire attempts"),
	)
	s.acquireDuration, _ = s.meter.Float64Histogram(
		"lockman.acquire.duration",
		metric.WithDescription("Time spent waiting to acquire a lock in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	s.contentionCount, _ = s.meter.Int64Counter(
		"lockman.contention.count",
		metric.WithDescription("Number of lock contention events"),
	)
	s.holdDuration, _ = s.meter.Float64Histogram(
		"lockman.hold.duration",
		metric.WithDescription("Duration a lock was held in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300),
	)
}

// Consume records the event through OpenTelemetry if providers are configured.
// Errors from providers are silently swallowed to maintain best-effort semantics.
func (s *OTelSink) Consume(ctx context.Context, event observe.Event) error {
	s.recordMetrics(ctx, event)
	s.recordSpan(ctx, event)
	return nil
}

func (s *OTelSink) recordMetrics(ctx context.Context, event observe.Event) {
	if s.meter == nil {
		return
	}

	attrs := s.commonAttributes(event)

	switch event.Kind {
	case observe.EventAcquireStarted, observe.EventAcquireSucceeded, observe.EventAcquireFailed:
		s.acquireTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
		if event.Wait > 0 {
			s.acquireDuration.Record(ctx, event.Wait.Seconds(), metric.WithAttributes(attrs...))
		}
	case observe.EventContention:
		s.contentionCount.Add(ctx, 1, metric.WithAttributes(attrs...))
	case observe.EventReleased:
		if event.Held > 0 {
			s.holdDuration.Record(ctx, event.Held.Seconds(), metric.WithAttributes(attrs...))
		}
	}
}

func (s *OTelSink) recordSpan(ctx context.Context, event observe.Event) {
	if s.tracer == nil {
		return
	}

	spanName := "lockman." + event.Kind.String()
	_, span := s.tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	attrs := s.commonAttributes(event)
	span.SetAttributes(attrs...)

	if event.Error != nil {
		span.RecordError(event.Error)
		span.SetStatus(codes.Error, event.Error.Error())
	}
}

func (s *OTelSink) commonAttributes(event observe.Event) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("lockman.definition_id", event.DefinitionID),
		attribute.String("lockman.request_id", event.RequestID),
		attribute.String("lockman.owner_id", event.OwnerID),
		attribute.String("lockman.resource_id", event.ResourceID),
		attribute.String("lockman.event_kind", event.Kind.String()),
		attribute.Bool("lockman.success", event.Success),
	}
	if event.Contention > 0 {
		attrs = append(attrs, attribute.Int("lockman.contention", event.Contention))
	}
	return attrs
}

var _ observe.Sink = (*OTelSink)(nil)
