package holds

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/registry"
)

// Manager orchestrates detached hold acquire and release operations.
type Manager struct {
	registry     registry.Reader
	driver       backend.Driver
	shuttingDown atomic.Bool
}

// NewManager validates dependencies and returns a configured hold manager.
func NewManager(reg registry.Reader, driver backend.Driver) (*Manager, error) {
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

	return &Manager{
		registry: reg,
		driver:   driver,
	}, nil
}

// Acquire claims the requested resource keys using detached hold request semantics.
func (m *Manager) Acquire(ctx context.Context, req definitions.DetachedAcquireRequest) (backend.LeaseRecord, error) {
	if m.shuttingDown.Load() {
		return backend.LeaseRecord{}, lockerrors.ErrPolicyViolation
	}

	def, err := m.getDefinition(req.DefinitionID)
	if err != nil {
		return backend.LeaseRecord{}, err
	}

	return m.driver.Acquire(ctx, backend.AcquireRequest{
		DefinitionID: req.DefinitionID,
		ResourceKeys: append([]string(nil), req.ResourceKeys...),
		OwnerID:      req.OwnerID,
		LeaseTTL:     def.LeaseTTL,
	})
}

// Release relinquishes a detached hold lease.
func (m *Manager) Release(ctx context.Context, req definitions.DetachedReleaseRequest) error {
	return m.driver.Release(ctx, backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: append([]string(nil), req.ResourceKeys...),
		OwnerID:      req.OwnerID,
	})
}

// Shutdown marks the manager unavailable for future acquires.
func (m *Manager) Shutdown() {
	m.shuttingDown.Store(true)
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
