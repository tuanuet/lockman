package inspect_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/observe"
)

func seedEvents(store *inspect.Store, count int) {
	for i := 0; i < count; i++ {
		_ = store.Consume(context.Background(), observe.Event{
			Kind:         observe.EventAcquireSucceeded,
			DefinitionID: "order.approve",
			ResourceID:   "order:1",
			OwnerID:      "api",
			Timestamp:    time.Now().Add(time.Duration(i) * time.Millisecond),
		})
	}
}

// ---------------------------------------------------------------------------
// GET /locks/inspect
// ---------------------------------------------------------------------------

func TestHandlerSnapshotEndpointReturnsJSON(t *testing.T) {
	store := inspect.NewStore()
	seedEvents(store, 3)
	store.UpdatePipelineState(inspect.PipelineState{BufferSize: 256})

	h := inspect.NewHandler(store)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/locks/inspect")
	if err != nil {
		t.Fatalf("GET /locks/inspect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var snap inspect.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snap.Pipeline.BufferSize != 256 {
		t.Fatalf("expected BufferSize=256, got %d", snap.Pipeline.BufferSize)
	}
}

// ---------------------------------------------------------------------------
// GET /locks/inspect/active
// ---------------------------------------------------------------------------

func TestHandlerActiveEndpointReturnsRuntimeLocks(t *testing.T) {
	store := inspect.NewStore()
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:1",
		OwnerID:      "api",
		Timestamp:    time.Now(),
	})

	h := inspect.NewHandler(store)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/locks/inspect/active")
	if err != nil {
		t.Fatalf("GET /locks/inspect/active: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var locks []inspect.RuntimeLockInfo
	if err := json.NewDecoder(resp.Body).Decode(&locks); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(locks) != 1 {
		t.Fatalf("expected 1 active lock, got %d", len(locks))
	}
}

// ---------------------------------------------------------------------------
// GET /locks/inspect/events
// ---------------------------------------------------------------------------

func TestHandlerEventsEndpointReturnsRecentEvents(t *testing.T) {
	store := inspect.NewStore()
	seedEvents(store, 5)

	h := inspect.NewHandler(store)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/locks/inspect/events")
	if err != nil {
		t.Fatalf("GET /locks/inspect/events: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var events []observe.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}
}

func TestHandlerEventsEndpointAcceptsLimitQuery(t *testing.T) {
	store := inspect.NewStore()
	seedEvents(store, 10)

	h := inspect.NewHandler(store)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/locks/inspect/events?limit=3")
	if err != nil {
		t.Fatalf("GET /locks/inspect/events?limit=3: %v", err)
	}
	defer resp.Body.Close()

	var events []observe.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
}

func TestHandlerEventsEndpointAcceptsDefinitionIDFilter(t *testing.T) {
	store := inspect.NewStore()

	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "order.approve",
		ResourceID:   "order:1",
		OwnerID:      "api",
		Timestamp:    time.Now(),
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:         observe.EventAcquireSucceeded,
		DefinitionID: "payment.process",
		ResourceID:   "pay:1",
		OwnerID:      "worker",
		Timestamp:    time.Now(),
	})

	h := inspect.NewHandler(store)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/locks/inspect/events?definition_id=order.approve")
	if err != nil {
		t.Fatalf("GET events with filter: %v", err)
	}
	defer resp.Body.Close()

	var events []observe.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// GET /locks/inspect/health
// ---------------------------------------------------------------------------

func TestHandlerHealthEndpointReturnsOK(t *testing.T) {
	store := inspect.NewStore()
	h := inspect.NewHandler(store)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/locks/inspect/health")
	if err != nil {
		t.Fatalf("GET /locks/inspect/health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %s", body["status"])
	}
}

// ---------------------------------------------------------------------------
// GET /locks/inspect/stream (SSE)
// ---------------------------------------------------------------------------

func TestHandlerStreamEndpointSetsSSEHeaders(t *testing.T) {
	store := inspect.NewStore()
	h := inspect.NewHandler(store)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/locks/inspect/stream")
	if err != nil {
		t.Fatalf("GET /locks/inspect/stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Fatalf("expected Content-Type=text/event-stream, got %s", ct)
	}
}

// ---------------------------------------------------------------------------
// 404 for unknown paths
// ---------------------------------------------------------------------------

func TestHandlerReturns404ForUnknownPath(t *testing.T) {
	store := inspect.NewStore()
	h := inspect.NewHandler(store)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/locks/inspect/nonexistent")
	if err != nil {
		t.Fatalf("GET unknown: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
