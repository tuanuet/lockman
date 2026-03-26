package workers

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	lockerrors "lockman/lockkit/errors"
	"lockman/lockkit/idempotency"
	"lockman/lockkit/registry"
)

type reentryKey struct {
	definitionID string
	resourceKey  string
	ownerID      string
}

type definitionSnapshotReader interface {
	Definitions() []definitions.LockDefinition
}

// Manager orchestrates single-resource worker claim execution for Phase 2.
type Manager struct {
	registry    registry.Reader
	driver      drivers.Driver
	idempotency idempotency.Store

	active sync.Map

	shuttingDown  atomic.Bool
	shutdownStart sync.Once
	lifecycleMu   sync.Mutex
	inFlight      int
	inFlightDrain chan struct{}

	renewalsMu  sync.Mutex
	renewals    map[uint64]context.CancelFunc
	nextRenewal uint64
}

// NewManager validates dependencies and returns a configured worker manager.
func NewManager(reg registry.Reader, driver drivers.Driver, store idempotency.Store) (*Manager, error) {
	if reg == nil {
		return nil, lockerrors.ErrRegistryViolation
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
	if err := driver.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("%w: %v", lockerrors.ErrPolicyViolation, err)
	}
	if store == nil && registryRequiresIdempotencyStore(reg) {
		return nil, lockerrors.ErrPolicyViolation
	}

	drain := make(chan struct{})
	close(drain)
	return &Manager{
		registry:      reg,
		driver:        driver,
		idempotency:   store,
		inFlightDrain: drain,
		renewals:      make(map[uint64]context.CancelFunc),
	}, nil
}

func registryRequiresIdempotencyStore(reg registry.Reader) bool {
	snapshot, ok := reg.(definitionSnapshotReader)
	if !ok {
		return false
	}

	for _, def := range snapshot.Definitions() {
		if !def.IdempotencyRequired {
			continue
		}
		if def.ExecutionKind == definitions.ExecutionAsync || def.ExecutionKind == definitions.ExecutionBoth {
			return true
		}
	}
	return false
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

func (m *Manager) registerRenewalCancel(cancel context.CancelFunc) uint64 {
	if cancel == nil {
		return 0
	}
	m.renewalsMu.Lock()
	defer m.renewalsMu.Unlock()
	m.nextRenewal++
	id := m.nextRenewal
	m.renewals[id] = cancel
	return id
}

func (m *Manager) unregisterRenewalCancel(id uint64) {
	if id == 0 {
		return
	}
	m.renewalsMu.Lock()
	defer m.renewalsMu.Unlock()
	delete(m.renewals, id)
}

func (m *Manager) cancelAllRenewals() {
	m.renewalsMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(m.renewals))
	for _, cancel := range m.renewals {
		cancels = append(cancels, cancel)
	}
	m.renewalsMu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}

func (m *Manager) getDefinition(id string) (def definitions.LockDefinition, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = lockerrors.ErrPolicyViolation
		}
	}()
	def = m.registry.MustGet(id)
	return def, err
}
