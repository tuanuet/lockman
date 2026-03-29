package workers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"lockman/backend"
	lockerrors "lockman/lockkit/errors"
)

const (
	minRenewInterval = 25 * time.Millisecond
	maxRenewInterval = 30 * time.Second
)

type renewalSession struct {
	stop func()
	done chan struct{}

	errMu sync.Mutex
	err   error
}

func (s *renewalSession) stopAndWait() {
	if s == nil {
		return
	}
	if s.stop != nil {
		s.stop()
	}
	if s.done != nil {
		<-s.done
	}
}

func (s *renewalSession) failure() error {
	if s == nil {
		return nil
	}
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

func (s *renewalSession) setFailure(err error) {
	if err == nil {
		return
	}
	s.errMu.Lock()
	defer s.errMu.Unlock()
	if s.err == nil {
		s.err = err
	}
}

func (m *Manager) startLeaseRenewal(
	lease renewableLease,
	onFailureCancel context.CancelFunc,
) *renewalSession {
	interval := renewalInterval(lease.lease.LeaseTTL)
	renewCtx, renewCancel := context.WithCancel(context.Background())
	session := &renewalSession{
		done: make(chan struct{}),
	}
	registrationID := m.registerRenewalCancel(renewCancel)
	session.stop = func() {
		renewCancel()
		m.unregisterRenewalCancel(registrationID)
	}

	go func() {
		defer close(session.done)
		defer m.unregisterRenewalCancel(registrationID)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		current := lease
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
			}

			updated, err := m.renewLease(renewCtx, current)
			if err != nil {
				if renewCtx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
					return
				}
				session.setFailure(fmt.Errorf("%w: %v", lockerrors.ErrLeaseLost, err))
				if onFailureCancel != nil {
					onFailureCancel()
				}
				return
			}
			current = updated
		}
	}()

	return session
}

func (m *Manager) renewLease(ctx context.Context, current renewableLease) (renewableLease, error) {
	if current.fencingToken > 0 {
		if current.lineage != nil {
			return renewableLease{}, lockerrors.ErrPolicyViolation
		}
		strictDriver, ok := m.driver.(backend.StrictDriver)
		if !ok {
			return renewableLease{}, lockerrors.ErrPolicyViolation
		}

		updated, err := strictDriver.RenewStrict(ctx, current.lease, current.fencingToken)
		if err != nil {
			return renewableLease{}, err
		}
		if updated.FencingToken != current.fencingToken {
			return renewableLease{}, backend.ErrLeaseNotFound
		}
		return renewableLease{
			lease:        updated.Lease,
			fencingToken: current.fencingToken,
		}, nil
	}

	if current.lineage == nil {
		updated, err := m.driver.Renew(ctx, current.lease)
		if err != nil {
			return renewableLease{}, err
		}
		return renewableLease{lease: updated}, nil
	}

	lineageDriver, ok := m.driver.(backend.LineageDriver)
	if !ok {
		return renewableLease{}, lockerrors.ErrPolicyViolation
	}

	updatedLease, updatedMeta, err := lineageDriver.RenewWithLineage(ctx, current.lease, cloneWorkerLineageMeta(*current.lineage))
	if err != nil {
		return renewableLease{}, err
	}
	meta := cloneWorkerLineageMeta(updatedMeta)
	return renewableLease{
		lease:   updatedLease,
		lineage: &meta,
	}, nil
}

func renewalInterval(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return minRenewInterval
	}
	interval := ttl / 3
	if interval < minRenewInterval {
		return minRenewInterval
	}
	if interval > maxRenewInterval {
		return maxRenewInterval
	}
	return interval
}
