package prometheus

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/tuanuet/lockman/observe"
)

func TestPrometheusSinkSatisfiesSinkInterface(t *testing.T) {
	var _ observe.Sink = NewPrometheusSink(PrometheusConfig{})
}

func TestPrometheusSinkConsumeNeverReturnsError(t *testing.T) {
	registry := prometheus.NewRegistry()
	sink := NewPrometheusSink(PrometheusConfig{Registerer: registry})

	err := sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "test-def",
		OwnerID:      "test-owner",
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func readCounterValue(registry *prometheus.Registry, name string, labels map[string]string) float64 {
	mf, err := registry.Gather()
	if err != nil {
		return -1
	}
	for _, m := range mf {
		if m.GetName() == name && len(m.Metric) > 0 {
			for _, c := range m.Metric {
				l := c.GetLabel()
				match := true
				for k, v := range labels {
					found := false
					for _, pl := range l {
						if pl.GetName() == k && pl.GetValue() == v {
							found = true
							break
						}
					}
					if !found {
						match = false
						break
					}
				}
				if match && c.Counter != nil {
					return c.Counter.GetValue()
				}
			}
		}
	}
	return -1
}

func readGaugeValue(registry *prometheus.Registry, name string, labels map[string]string) float64 {
	mf, err := registry.Gather()
	if err != nil {
		return -1
	}
	for _, m := range mf {
		if m.GetName() == name && len(m.Metric) > 0 {
			for _, c := range m.Metric {
				l := c.GetLabel()
				match := true
				for k, v := range labels {
					found := false
					for _, pl := range l {
						if pl.GetName() == k && pl.GetValue() == v {
							found = true
							break
						}
					}
					if !found {
						match = false
						break
					}
				}
				if match && c.Gauge != nil {
					return c.Gauge.GetValue()
				}
			}
		}
	}
	return -1
}

func TestPrometheusSinkConsumeAcquireSucceeded(t *testing.T) {
	registry := prometheus.NewRegistry()
	sink := NewPrometheusSink(PrometheusConfig{Registerer: registry})

	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
		Wait:         100 * time.Millisecond,
	})

	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "def-1",
		OwnerID:      "owner-2",
	})

	val := readCounterValue(registry, "lockman_acquire_total", map[string]string{"definition_id": "def-1", "outcome": "success"})
	if val != 2 {
		t.Errorf("expected acquire_total=2, got %v", val)
	}

	val = readGaugeValue(registry, "lockman_active_locks", map[string]string{"definition_id": "def-1", "owner_id": "owner-1"})
	if val != 1 {
		t.Errorf("expected active_locks{owner-1}=1, got %v", val)
	}

	val = readGaugeValue(registry, "lockman_active_locks", map[string]string{"definition_id": "def-1", "owner_id": "owner-2"})
	if val != 1 {
		t.Errorf("expected active_locks{owner-2}=1, got %v", val)
	}
}

func TestPrometheusSinkConsumeAcquireFailed(t *testing.T) {
	registry := prometheus.NewRegistry()
	sink := NewPrometheusSink(PrometheusConfig{Registerer: registry})

	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireFailed,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
		Wait:         50 * time.Millisecond,
	})

	val := readCounterValue(registry, "lockman_acquire_total", map[string]string{"definition_id": "def-1", "outcome": "failure"})
	if val != 1 {
		t.Errorf("expected acquire_total=1, got %v", val)
	}
}

func TestPrometheusSinkConsumeReleased(t *testing.T) {
	registry := prometheus.NewRegistry()
	sink := NewPrometheusSink(PrometheusConfig{Registerer: registry})

	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
	})

	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventReleased,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
		Held:         5 * time.Second,
	})

	val := readGaugeValue(registry, "lockman_active_locks", map[string]string{"definition_id": "def-1", "owner_id": "owner-1"})
	if val != 0 {
		t.Errorf("expected active_locks=0, got %v", val)
	}
}

func TestPrometheusSinkConsumeContention(t *testing.T) {
	registry := prometheus.NewRegistry()
	sink := NewPrometheusSink(PrometheusConfig{Registerer: registry})

	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventContention,
		DefinitionID: "def-1",
		Contention:   2,
	})

	val := readCounterValue(registry, "lockman_contention_total", map[string]string{"definition_id": "def-1"})
	if val != 1 {
		t.Errorf("expected contention_total=1, got %v", val)
	}
}

func TestPrometheusSinkConsumeLeaseLost(t *testing.T) {
	registry := prometheus.NewRegistry()
	sink := NewPrometheusSink(PrometheusConfig{Registerer: registry})

	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
	})

	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventLeaseLost,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
	})

	val := readGaugeValue(registry, "lockman_active_locks", map[string]string{"definition_id": "def-1", "owner_id": "owner-1"})
	if val != 0 {
		t.Errorf("expected active_locks=0, got %v", val)
	}
}

func TestPrometheusSinkConsumeRenewalSucceeded(t *testing.T) {
	registry := prometheus.NewRegistry()
	sink := NewPrometheusSink(PrometheusConfig{Registerer: registry})

	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventRenewalSucceeded,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
	})

	val := readCounterValue(registry, "lockman_renewal_total", map[string]string{"definition_id": "def-1", "outcome": "success"})
	if val != 1 {
		t.Errorf("expected renewal_total=1, got %v", val)
	}
}

func TestPrometheusSinkConsumeRenewalFailed(t *testing.T) {
	registry := prometheus.NewRegistry()
	sink := NewPrometheusSink(PrometheusConfig{Registerer: registry})

	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventRenewalFailed,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
	})

	val := readCounterValue(registry, "lockman_renewal_total", map[string]string{"definition_id": "def-1", "outcome": "failure"})
	if val != 1 {
		t.Errorf("expected renewal_total=1, got %v", val)
	}
}

func TestPrometheusSinkIgnoresUnknownEvents(t *testing.T) {
	registry := prometheus.NewRegistry()
	sink := NewPrometheusSink(PrometheusConfig{Registerer: registry})

	err := sink.Consume(context.Background(), observe.Event{
		Kind:         999,
		DefinitionID: "def-1",
	})

	if err != nil {
		t.Errorf("expected nil error for unknown event, got %v", err)
	}

	mf, _ := registry.Gather()
	if len(mf) != 0 {
		t.Errorf("expected no metrics for unknown event, got %d", len(mf))
	}
}

func TestPrometheusSinkActiveLocksMultipleOwners(t *testing.T) {
	registry := prometheus.NewRegistry()
	sink := NewPrometheusSink(PrometheusConfig{Registerer: registry})

	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
	})
	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "def-1",
		OwnerID:      "owner-2",
	})
	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
	})

	val := readGaugeValue(registry, "lockman_active_locks", map[string]string{"definition_id": "def-1", "owner_id": "owner-1"})
	if val != 2 {
		t.Errorf("expected active_locks{owner-1}=2, got %v", val)
	}

	val = readGaugeValue(registry, "lockman_active_locks", map[string]string{"definition_id": "def-1", "owner_id": "owner-2"})
	if val != 1 {
		t.Errorf("expected active_locks{owner-2}=1, got %v", val)
	}
}

func TestPrometheusSinkActiveLocksCleansUpZeroCount(t *testing.T) {
	registry := prometheus.NewRegistry()
	sink := NewPrometheusSink(PrometheusConfig{Registerer: registry})

	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
	})
	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventReleased,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
		Held:         1 * time.Second,
	})
	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
	})
	sink.Consume(context.Background(), observe.Event{
		Kind:         observe.EventReleased,
		DefinitionID: "def-1",
		OwnerID:      "owner-1",
		Held:         1 * time.Second,
	})

	val := readGaugeValue(registry, "lockman_active_locks", map[string]string{"definition_id": "def-1", "owner_id": "owner-1"})
	if val != 0 {
		t.Errorf("expected active_locks=0, got %v", val)
	}
}
