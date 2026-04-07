package prometheus

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tuanuet/lockman/observe"
)

type PrometheusConfig struct {
	Registerer prometheus.Registerer
	Namespace  string
	Subsystem  string
}

type activeKey struct {
	definitionID string
	ownerID      string
}

type PrometheusSink struct {
	acquireTotal    *prometheus.CounterVec
	acquireDuration *prometheus.HistogramVec
	holdDuration    *prometheus.HistogramVec
	contentionTotal *prometheus.CounterVec
	activeLocks     *prometheus.GaugeVec
	renewalTotal    *prometheus.CounterVec

	mu             sync.Mutex
	activeLocksMap map[activeKey]int
}

func NewPrometheusSink(cfg PrometheusConfig) observe.Sink {
	namespace := cfg.Namespace
	if namespace == "" {
		namespace = "lockman"
	}
	subsystem := cfg.Subsystem

	reg := cfg.Registerer
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	s := &PrometheusSink{
		activeLocksMap: make(map[activeKey]int),
	}

	s.acquireTotal = promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "acquire_total",
			Help:      "Total number of lock acquire attempts",
		},
		[]string{"definition_id", "outcome"},
	)

	s.acquireDuration = promauto.With(reg).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "acquire_duration_seconds",
			Help:      "Time spent waiting to acquire a lock in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"definition_id"},
	)

	s.holdDuration = promauto.With(reg).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "hold_duration_seconds",
			Help:      "Duration a lock was held in seconds",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300},
		},
		[]string{"definition_id"},
	)

	s.contentionTotal = promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "contention_total",
			Help:      "Number of lock contention events",
		},
		[]string{"definition_id"},
	)

	s.activeLocks = promauto.With(reg).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "active_locks",
			Help:      "Number of currently active locks",
		},
		[]string{"definition_id", "owner_id"},
	)

	s.renewalTotal = promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "renewal_total",
			Help:      "Total number of lock renewal attempts",
		},
		[]string{"definition_id", "outcome"},
	)

	return s
}

func (s *PrometheusSink) Consume(ctx context.Context, event observe.Event) error {
	defID := event.DefinitionID

	switch event.Kind {
	case observe.EventAcquireSucceeded:
		s.acquireTotal.WithLabelValues(defID, "success").Inc()
		if event.Wait > 0 {
			s.acquireDuration.WithLabelValues(defID).Observe(event.Wait.Seconds())
		}
		s.incrementActiveLock(defID, event.OwnerID)

	case observe.EventAcquireFailed:
		s.acquireTotal.WithLabelValues(defID, "failure").Inc()
		if event.Wait > 0 {
			s.acquireDuration.WithLabelValues(defID).Observe(event.Wait.Seconds())
		}

	case observe.EventReleased:
		if event.Held > 0 {
			s.holdDuration.WithLabelValues(defID).Observe(event.Held.Seconds())
		}
		s.decrementActiveLock(defID, event.OwnerID)

	case observe.EventContention:
		s.contentionTotal.WithLabelValues(defID).Inc()

	case observe.EventLeaseLost:
		s.decrementActiveLock(defID, event.OwnerID)

	case observe.EventRenewalSucceeded:
		s.renewalTotal.WithLabelValues(defID, "success").Inc()

	case observe.EventRenewalFailed:
		s.renewalTotal.WithLabelValues(defID, "failure").Inc()
	}

	return nil
}

func (s *PrometheusSink) incrementActiveLock(defID, ownerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := activeKey{definitionID: defID, ownerID: ownerID}
	s.activeLocksMap[key]++
	s.activeLocks.WithLabelValues(defID, ownerID).Set(float64(s.activeLocksMap[key]))
}

func (s *PrometheusSink) decrementActiveLock(defID, ownerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := activeKey{definitionID: defID, ownerID: ownerID}
	if s.activeLocksMap[key] > 0 {
		s.activeLocksMap[key]--
	}
	if s.activeLocksMap[key] == 0 {
		s.activeLocks.WithLabelValues(defID, ownerID).Set(0)
	} else {
		s.activeLocks.WithLabelValues(defID, ownerID).Set(float64(s.activeLocksMap[key]))
	}
}

var _ observe.Sink = (*PrometheusSink)(nil)
