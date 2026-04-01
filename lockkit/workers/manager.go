package workers

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/idempotency"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/observe"
)

type reentryKey struct {
	definitionID string
	resourceKey  string
}

type definitionSnapshotReader interface {
	Definitions() []definitions.LockDefinition
}

// Bridge receives worker lifecycle events from the manager.
type Bridge interface {
	PublishWorkerAcquireStarted(e observe.Event)
	PublishWorkerAcquireSucceeded(e observe.Event)
	PublishWorkerAcquireFailed(e observe.Event, err error)
	PublishWorkerReleased(e observe.Event)
	PublishWorkerOverlap(e observe.Event)
	PublishWorkerRenewalSucceeded(e observe.Event)
	PublishWorkerLeaseLost(e observe.Event)
	PublishWorkerShutdownStarted()
	PublishWorkerShutdownCompleted()
}

// Option configures the worker manager.
type Option func(*managerConfig)

type managerConfig struct {
	bridge Bridge
}

// WithBridge attaches an observability bridge to the worker manager.
func WithBridge(b Bridge) Option {
	return func(cfg *managerConfig) {
		cfg.bridge = b
	}
}

// Manager orchestrates single-resource worker claim execution for Phase 2.
type Manager struct {
	registry    registry.Reader
	driver      backend.Driver
	idempotency idempotency.Store
	bridge      Bridge

	active sync.Map

	shuttingDown  atomic.Bool
	shutdownStart sync.Once
	lifecycleMu   sync.Mutex
	inFlight      int
	inFlightDrain chan struct{}

	renewalsMu  sync.Mutex
	renewals    map[uint64]context.CancelFunc
	nextRenewal uint64

	lineageDefs    map[string]bool
	cachedDefsByID map[string]definitions.LockDefinition
}

// NewManager validates dependencies and returns a configured worker manager.
func NewManager(reg registry.Reader, driver backend.Driver, store idempotency.Store, opts ...Option) (*Manager, error) {
	var cfg managerConfig
	for _, opt := range opts {
		opt(&cfg)
	}

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
	if registry.RequiresLineageDriver(reg) {
		if _, ok := driver.(backend.LineageDriver); !ok {
			return nil, lockerrors.ErrPolicyViolation
		}
	}
	if registry.RequiresStrictWorkerDriver(reg) {
		if _, ok := driver.(backend.StrictDriver); !ok {
			return nil, lockerrors.ErrPolicyViolation
		}
	}
	if store == nil && registryRequiresIdempotencyStore(reg) {
		return nil, lockerrors.ErrPolicyViolation
	}

	drain := make(chan struct{})
	close(drain)

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
		registry:       reg,
		driver:         driver,
		idempotency:    store,
		bridge:         cfg.bridge,
		inFlightDrain:  drain,
		renewals:       make(map[uint64]context.CancelFunc),
		lineageDefs:    lineageDefs,
		cachedDefsByID: defsByID,
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

func (m *Manager) getDefinition(id string) (definitions.LockDefinition, bool) {
	return m.registry.Get(id)
}
