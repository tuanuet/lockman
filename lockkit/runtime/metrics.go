package runtime

import (
	"context"
	"time"
)

func (m *Manager) recordPresenceCheck(ctx context.Context, definitionID string, started time.Time) {
	m.recorder.RecordPresenceCheck(ctx, definitionID, time.Since(started))
}
