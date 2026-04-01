package observe

import (
	"testing"
)

func TestEventKindStrings(t *testing.T) {
	tests := []struct {
		kind     EventKind
		expected string
	}{
		{EventAcquireStarted, "acquire_started"},
		{EventAcquireSucceeded, "acquire_succeeded"},
		{EventAcquireFailed, "acquire_failed"},
		{EventReleased, "released"},
		{EventContention, "contention"},
		{EventOverlap, "overlap"},
		{EventLeaseLost, "lease_lost"},
		{EventRenewalSucceeded, "renewal_succeeded"},
		{EventRenewalFailed, "renewal_failed"},
		{EventShutdownStarted, "shutdown_started"},
		{EventShutdownCompleted, "shutdown_completed"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.kind.String() != tt.expected {
				t.Errorf("EventKind(%d).String() = %q, want %q", tt.kind, tt.kind.String(), tt.expected)
			}
		})
	}
}

func TestEventKindIsValid(t *testing.T) {
	if !EventAcquireStarted.IsValid() {
		t.Error("EventAcquireStarted should be valid")
	}
	if EventKind(999).IsValid() {
		t.Error("EventKind(999) should be invalid")
	}
}
