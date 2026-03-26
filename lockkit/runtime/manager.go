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
	}, nil
}

// Shutdown marks the manager as unavailable for new lock acquisitions.
func (m *Manager) Shutdown(ctx context.Context) error {
	_ = ctx
	m.shutdownStart.Do(func() {
		m.shuttingDown.Store(true)
	})
	return nil
}

func (m *Manager) isShuttingDown() bool {
	return m.shuttingDown.Load()
}
