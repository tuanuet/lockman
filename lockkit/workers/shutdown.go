package workers

import (
	"context"
	"sync"
)

// Shutdown marks the worker manager unavailable for new claims and waits for in-flight executions to drain.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.shutdownStart.Do(func() {
		m.shuttingDown.Store(true)
		if m.bridge != nil {
			m.bridge.PublishWorkerShutdownStarted()
		}
	})

	m.drainMu.Lock()
	defer m.drainMu.Unlock()
	for m.inFlight.Load() > 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if done := waitForDrainWithContext(m.drainCond, &m.drainMu, func() bool {
			return m.inFlight.Load() == 0
		}, ctx.Done()); !done {
			return ctx.Err()
		}
	}
	if m.bridge != nil {
		m.bridge.PublishWorkerShutdownCompleted()
	}
	return nil
}

func waitForDrainWithContext(cond *sync.Cond, mu *sync.Mutex, drained func() bool, done <-chan struct{}) bool {
	if drained() {
		return true
	}

	if done == nil {
		for !drained() {
			cond.Wait()
		}
		return true
	}

	stop := make(chan struct{})
	defer close(stop)

	go func() {
		select {
		case <-done:
			mu.Lock()
			cond.Broadcast()
			mu.Unlock()
		case <-stop:
		}
	}()

	for !drained() {
		cond.Wait()
		select {
		case <-done:
			if !drained() {
				return false
			}
		default:
		}
	}

	return true
}
