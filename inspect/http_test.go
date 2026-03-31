package inspect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tuanuet/lockman/observe"
)

func TestHTTPHandlerLocksInspect(t *testing.T) {
	store := NewStore()
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock1",
		ResourceID:   "res:1",
		OwnerID:      "owner1",
		Timestamp:    time.Now(),
	})

	handler := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/locks/inspect", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var snap Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(snap.RuntimeLocks) != 1 {
		t.Fatalf("expected 1 runtime lock, got %d", len(snap.RuntimeLocks))
	}
}

func TestHTTPHandlerActiveLocks(t *testing.T) {
	store := NewStore()
	_ = store.Consume(nil, observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock1",
		ResourceID:   "res:1",
		OwnerID:      "owner1",
		Timestamp:    time.Now(),
	})

	handler := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/locks/inspect/active", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["runtime_locks"] == nil {
		t.Fatal("expected runtime_locks key")
	}
}

func TestHTTPHandlerEvents(t *testing.T) {
	store := NewStore()
	_ = store.Consume(nil, observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock1",
		ResourceID:   "res:1",
		Timestamp:    time.Now(),
	})

	handler := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/locks/inspect/events", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var events []Event
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestHTTPHandlerHealth(t *testing.T) {
	store := NewStore()
	store.UpdatePipelineState(PipelineState{
		Status:     "healthy",
		QueueDepth: 10,
		Errors:     0,
	})

	handler := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/locks/inspect/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var health map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &health); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if health["status"] != "healthy" {
		t.Fatalf("expected healthy status, got %v", health["status"])
	}
}

func TestHTTPHandlerStream(t *testing.T) {
	store := NewStore()

	handler := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/locks/inspect/stream", nil)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(w, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("handler timed out")
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", contentType)
	}
}

func TestHTTPHandlerEventsWithFilters(t *testing.T) {
	store := NewStore()
	now := time.Now()
	_ = store.Consume(nil, observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "lock1",
		ResourceID:   "res:1",
		OwnerID:      "owner1",
		Timestamp:    now,
	})
	_ = store.Consume(nil, observe.Event{
		Kind:         observe.EventReleased,
		DefinitionID: "lock1",
		ResourceID:   "res:1",
		OwnerID:      "owner1",
		Timestamp:    now,
	})

	handler := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/locks/inspect/events?kind=acquire_succeeded", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var events []Event
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 filtered event, got %d", len(events))
	}
}

func TestHTTPHandler404(t *testing.T) {
	store := NewStore()

	handler := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}
