package observe

import (
	"context"
)

type NoopSink struct{}

func (NoopSink) Consume(ctx context.Context, event Event) error {
	return nil
}

type NoopExporter struct{}

func (NoopExporter) Export(ctx context.Context, event Event) error {
	return nil
}

type NoopDispatcher struct{}

func (NoopDispatcher) Publish(event Event) {}

func (NoopDispatcher) Shutdown(ctx context.Context) error {
	return nil
}

func (NoopDispatcher) DroppedCount() int64 {
	return 0
}

func (NoopDispatcher) SinkFailureCount() int64 {
	return 0
}

func (NoopDispatcher) ExporterFailureCount() int64 {
	return 0
}

var _ Sink = NoopSink{}
var _ Exporter = NoopExporter{}
var _ Dispatcher = NoopDispatcher{}
