package workers

import "context"

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
		done := make(chan struct{})
		go func() {
			m.drainMu.Lock()
			m.drainCond.Wait()
			m.drainMu.Unlock()
			close(done)
		}()
		m.drainMu.Unlock()
		select {
		case <-done:
			m.drainMu.Lock()
		case <-ctx.Done():
			m.drainCond.Broadcast()
			<-done
			m.drainMu.Lock()
			return ctx.Err()
		}
	}
	if m.bridge != nil {
		m.bridge.PublishWorkerShutdownCompleted()
	}
	return nil
}
