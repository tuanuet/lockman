package observe

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type dispatcher struct {
	eventCh     chan Event
	sinks       []Sink
	exporters   []Exporter
	dropPolicy  DropPolicy
	workerCount int

	droppedCount         atomic.Int64
	sinkFailureCount     atomic.Int64
	exporterFailureCount atomic.Int64

	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

func NewDispatcher(opts ...Option) *dispatcher {
	cfg := buildConfig(opts)

	d := &dispatcher{
		eventCh:     make(chan Event, cfg.bufferSize),
		sinks:       cfg.sinks,
		exporters:   cfg.exporters,
		dropPolicy:  cfg.dropPolicy,
		workerCount: cfg.workerCount,
	}

	for i := 0; i < d.workerCount; i++ {
		d.wg.Add(1)
		go d.worker()
	}

	return d
}

func (d *dispatcher) Publish(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	select {
	case d.eventCh <- event:
		return
	default:
		d.handleDrop(event)
	}
}

func (d *dispatcher) handleDrop(event Event) {
	d.droppedCount.Add(1)

	switch d.dropPolicy {
	case DropPolicyDropOldest:
		select {
		case <-d.eventCh:
		default:
		}
		select {
		case d.eventCh <- event:
		default:
		}
	case DropPolicyDropNewest:
		// Just drop the new event
	}
}

func (d *dispatcher) worker() {
	defer d.wg.Done()

	for event := range d.eventCh {
		d.deliverToSinks(event)
		d.deliverToExporters(event)
	}
}

func (d *dispatcher) deliverToSinks(event Event) {
	var wg sync.WaitGroup
	for _, sink := range d.sinks {
		sink := sink
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sink.Consume(context.Background(), event); err != nil {
				d.sinkFailureCount.Add(1)
			}
		}()
	}
	wg.Wait()
}

func (d *dispatcher) deliverToExporters(event Event) {
	var wg sync.WaitGroup
	for _, exp := range d.exporters {
		exp := exp
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := exp.Export(context.Background(), event); err != nil {
				d.exporterFailureCount.Add(1)
			}
		}()
	}
	wg.Wait()
}

func (d *dispatcher) Shutdown(ctx context.Context) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	close(d.eventCh)
	d.mu.Unlock()

	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *dispatcher) DroppedCount() int64 {
	return d.droppedCount.Load()
}

func (d *dispatcher) SinkFailureCount() int64 {
	return d.sinkFailureCount.Load()
}

func (d *dispatcher) ExporterFailureCount() int64 {
	return d.exporterFailureCount.Load()
}
