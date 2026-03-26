package runtime

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"lockman/lockkit/drivers"
	lockerrors "lockman/lockkit/errors"
	"lockman/lockkit/observe"
	"lockman/lockkit/registry"
)

// Manager orchestrates standard exclusive lock execution for Phase 1.
type Manager struct {
	registry      registry.Reader
	driver        drivers.Driver
	recorder      observe.Recorder
	active        sync.Map
	shuttingDown  atomic.Bool
	shutdownStart sync.Once
	lifecycleMu   sync.Mutex
	inFlight      int
	inFlightDrain chan struct{}
}

// NewManager validates the registry and returns a configured runtime manager.
func NewManager(reg registry.Reader, driver drivers.Driver, recorder observe.Recorder) (*Manager, error) {
	validator, ok := reg.(interface{ Validate() error })
	if !ok {
		return nil, fmt.Errorf("%w: invalid registry", lockerrors.ErrRegistryViolation)
	}
	if err := validator.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", lockerrors.ErrRegistryViolation, err)
	}
	if recorder == nil {
		recorder = observe.NewNoopRecorder()
	}
	return &Manager{
		registry: reg,
		driver:   driver,
		recorder: recorder,
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
		m.shuttingDown.Store(true)
	})

	drained := m.inFlightDrainChannel()
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) isShuttingDown() bool {
	return m.shuttingDown.Load()
}

func (m *Manager) admitInFlightExecution() {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	if m.inFlight == 0 {
		m.inFlightDrain = make(chan struct{})
	}
	m.inFlight++
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
