package observe

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
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

func TestEventJSONRoundTrip(t *testing.T) {
	testErr := errors.New("lock timeout")
	evt := Event{
		Kind:         EventAcquireFailed,
		DefinitionID: "order",
		ResourceID:   "order:123",
		OwnerID:      "api-1",
		Timestamp:    time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC),
		Error:        testErr,
	}

	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Kind != evt.Kind {
		t.Errorf("kind = %v, want %v", got.Kind, evt.Kind)
	}
	if got.DefinitionID != evt.DefinitionID {
		t.Errorf("definition_id = %q, want %q", got.DefinitionID, evt.DefinitionID)
	}
	if got.ResourceID != evt.ResourceID {
		t.Errorf("resource_id = %q, want %q", got.ResourceID, evt.ResourceID)
	}
	if got.OwnerID != evt.OwnerID {
		t.Errorf("owner_id = %q, want %q", got.OwnerID, evt.OwnerID)
	}
}
