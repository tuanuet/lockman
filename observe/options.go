package observe

import "context"

type DropPolicy int

const (
	DropPolicyDropOldest DropPolicy = iota
	DropPolicyDropNewest
)

type Config struct {
	bufferSize  int
	dropPolicy  DropPolicy
	sinks       []Sink
	exporters   []Exporter
	workerCount int
}

type Option func(*Config)

func WithBufferSize(size int) Option {
	return func(c *Config) {
		c.bufferSize = size
	}
}

func WithDropPolicy(policy DropPolicy) Option {
	return func(c *Config) {
		c.dropPolicy = policy
	}
}

func WithSink(sink Sink) Option {
	return func(c *Config) {
		c.sinks = append(c.sinks, sink)
	}
}

func WithExporter(exporter Exporter) Option {
	return func(c *Config) {
		c.exporters = append(c.exporters, exporter)
	}
}

func WithWorkerCount(count int) Option {
	return func(c *Config) {
		c.workerCount = count
	}
}

func buildConfig(opts []Option) *Config {
	cfg := &Config{
		bufferSize:  100,
		dropPolicy:  DropPolicyDropOldest,
		workerCount: 4,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

type Dispatcher interface {
	Publish(event Event)
	Shutdown(ctx context.Context) error
	DroppedCount() int64
	SinkFailureCount() int64
	ExporterFailureCount() int64
}
