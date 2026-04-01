package workers

import "context"

// Shutdown marks the worker manager unavailable for new claims and waits for in-flight executions to drain.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.shutdownStart.Do(func() {
		m.lifecycleMu.Lock()
		m.shuttingDown.Store(true)
		m.lifecycleMu.Unlock()
		if m.bridge != nil {
			m.bridge.PublishWorkerShutdownStarted()
		}
	})

	drained := m.inFlightDrainChannel()
	select {
	case <-drained:
		m.cancelAllRenewals()
		if m.bridge != nil {
			m.bridge.PublishWorkerShutdownCompleted()
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
