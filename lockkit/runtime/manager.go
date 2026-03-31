package runtime

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/observe"
	"github.com/tuanuet/lockman/lockkit/registry"
)

// RuntimeEvent carries the fields for a runtime lifecycle event emitted via Bridge.
type RuntimeEvent struct {
	DefinitionID string
	ResourceID   string
	OwnerID      string
	RequestID    string
	Wait         time.Duration
	Held         time.Duration
	Contention   int
}

// Bridge receives runtime lifecycle events from the manager.
type Bridge interface {
	PublishRuntimeAcquireStarted(re RuntimeEvent)
	PublishRuntimeAcquireSucceeded(re RuntimeEvent)
	PublishRuntimeAcquireFailed(re RuntimeEvent, err error)
	PublishRuntimeContention(re RuntimeEvent)
	PublishRuntimeOverlapRejected(re RuntimeEvent)
	PublishRuntimeReleased(re RuntimeEvent)
	PublishRuntimePresenceChecked(re RuntimeEvent)
	PublishRuntimeShutdownStarted()
	PublishRuntimeShutdownCompleted()
}

// Option configures the runtime manager.
type Option func(*managerConfig)

type managerConfig struct {
	bridge Bridge
}

// WithBridge attaches an observability bridge to the runtime manager.
func WithBridge(b Bridge) Option {
	return func(cfg *managerConfig) {
		cfg.bridge = b
	}
}

// Manager orchestrates standard exclusive lock execution for Phase 1.
type Manager struct {
	registry      registry.Reader
	driver        backend.Driver
	recorder      observe.Recorder
	bridge        Bridge
	active        sync.Map
	shuttingDown  atomic.Bool
	shutdownStart sync.Once
	lifecycleMu   sync.Mutex
	inFlight      int
	inFlightDrain chan struct{}
}

// NewManager validates the registry and returns a configured runtime manager.
func NewManager(reg registry.Reader, driver backend.Driver, recorder observe.Recorder, opts ...Option) (*Manager, error) {
	var cfg managerConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	validator, ok := reg.(interface{ Validate() error })
	if !ok {
		return nil, fmt.Errorf("%w: invalid registry", lockerrors.ErrRegistryViolation)
	}
	if err := validator.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", lockerrors.ErrRegistryViolation, err)
	}
	if driver == nil {
		return nil, lockerrors.ErrPolicyViolation
	}
	if registry.RequiresLineageDriver(reg) {
		if _, ok := driver.(backend.LineageDriver); !ok {
			return nil, lockerrors.ErrPolicyViolation
		}
	}
	if registry.RequiresStrictRuntimeDriver(reg) {
		if _, ok := driver.(backend.StrictDriver); !ok {
			return nil, lockerrors.ErrPolicyViolation
		}
	}
	if recorder == nil {
		recorder = observe.NewNoopRecorder()
	}
	return &Manager{
		registry: reg,
		driver:   driver,
		recorder: recorder,
		bridge:   cfg.bridge,
		inFlightDrain: func() chan struct{} {
			ch := make(chan struct{})
			close(ch)
			return ch
		}(),
	}, nil
}

// Shutdown marks the manager as unavailable for new lock acquisitions and
// waits for admitted in-flight executions to drain.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.shutdownStart.Do(func() {
		m.lifecycleMu.Lock()
		m.shuttingDown.Store(true)
		m.lifecycleMu.Unlock()
		if m.bridge != nil {
			m.bridge.PublishRuntimeShutdownStarted()
		}
	})

	drained := m.inFlightDrainChannel()
	select {
	case <-drained:
		if m.bridge != nil {
			m.bridge.PublishRuntimeShutdownCompleted()
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) isShuttingDown() bool {
	return m.shuttingDown.Load()
}

func (m *Manager) tryAdmitInFlightExecution() bool {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	if m.shuttingDown.Load() {
		return false
	}

	if m.inFlight == 0 {
		m.inFlightDrain = make(chan struct{})
	}
	m.inFlight++
	return true
}

func (m *Manager) releaseInFlightExecution() {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	if m.inFlight <= 0 {
		return
	}

	m.inFlight--
	if m.inFlight == 0 {
		close(m.inFlightDrain)
	}
}

func (m *Manager) inFlightDrainChannel() <-chan struct{} {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	return m.inFlightDrain
}

func (m *Manager) getCompositeDefinition(id string) (def definitions.CompositeDefinition, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = lockerrors.ErrPolicyViolation
		}
	}()
	def = m.registry.MustGetComposite(id)
	return def, err
}
