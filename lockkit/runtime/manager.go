package runtime

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	lockobserve "github.com/tuanuet/lockman/lockkit/observe"
	"github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/observe"
)

// Bridge receives runtime lifecycle events from the manager.
type Bridge interface {
	PublishRuntimeAcquireStarted(re observe.Event)
	PublishRuntimeAcquireSucceeded(re observe.Event)
	PublishRuntimeAcquireFailed(re observe.Event, err error)
	PublishRuntimeContention(re observe.Event)
	PublishRuntimeOverlapRejected(re observe.Event)
	PublishRuntimeReleased(re observe.Event)
	PublishRuntimePresenceChecked(re observe.Event)
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
	recorder      lockobserve.Recorder
	bridge        Bridge
	active        sync.Map
	activeByDef   sync.Map // definitionID → *atomic.Int64
	shuttingDown  atomic.Bool
	shutdownStart sync.Once
	lifecycleMu   sync.Mutex
	inFlight      int
	inFlightDrain chan struct{}

	lineageDefs    map[string]bool
	cachedDefsByID map[string]definitions.LockDefinition
}

// NewManager validates the registry and returns a configured runtime manager.
func NewManager(reg registry.Reader, driver backend.Driver, recorder lockobserve.Recorder, opts ...Option) (*Manager, error) {
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
		recorder = lockobserve.NewNoopRecorder()
	}

	defs := reg.Definitions()
	defsByID := make(map[string]definitions.LockDefinition, len(defs))
	for _, def := range defs {
		defsByID[def.ID] = def
	}
	childrenByParent := make(map[string][]string, len(defs))
	for _, def := range defs {
		if def.ParentRef == "" {
			continue
		}
		childrenByParent[def.ParentRef] = append(childrenByParent[def.ParentRef], def.ID)
	}
	lineageDefs := make(map[string]bool, len(defs))
	for _, def := range defs {
		lineageDefs[def.ID] = def.ParentRef != "" || len(childrenByParent[def.ID]) > 0
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
		lineageDefs:    lineageDefs,
		cachedDefsByID: defsByID,
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

func (m *Manager) getCompositeDefinition(id string) (definitions.CompositeDefinition, bool) {
	return m.registry.GetComposite(id)
}

func (m *Manager) activeCounter(definitionID string) *atomic.Int64 {
	if v, ok := m.activeByDef.Load(definitionID); ok {
		return v.(*atomic.Int64)
	}
	counter := &atomic.Int64{}
	actual, _ := m.activeByDef.LoadOrStore(definitionID, counter)
	return actual.(*atomic.Int64)
}
