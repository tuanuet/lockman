package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/observe"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func TestClient_Snapshot(t *testing.T) {
	snap := inspect.Snapshot{
		RuntimeLocks: []inspect.RuntimeLockInfo{
			{DefinitionID: "order", ResourceID: "order:123", OwnerID: "api-1", AcquiredAt: time.Now()},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, snap)
	}))
	defer srv.Close()

	c := New(srv.URL)
	got, err := c.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RuntimeLocks) != 1 {
		t.Fatalf("expected 1 lock, got %d", len(got.RuntimeLocks))
	}
}

func TestClient_Active(t *testing.T) {
	locks := []inspect.RuntimeLockInfo{
		{DefinitionID: "order", ResourceID: "order:1", OwnerID: "api-1", AcquiredAt: time.Now()},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, locks)
	}))
	defer srv.Close()

	c := New(srv.URL)
	got, err := c.Active(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 lock, got %d", len(got))
	}
}

func TestClient_Active_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := New(srv.URL)
	got, err := c.Active(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 locks, got %d", len(got))
	}
}

func TestClient_Events_FilterParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("definition_id"); got != "order" {
			t.Errorf("definition_id = %q, want %q", got, "order")
		}
		if got := r.URL.Query().Get("kind"); got != "contention" {
			t.Errorf("kind = %q, want %q", got, "contention")
		}
		writeJSON(w, []observe.Event{})
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.Events(context.Background(), Filter{
		DefinitionID: "order",
		Kind:         observe.EventContention,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_Events_KindMapping(t *testing.T) {
	tests := []struct {
		input string
		want  observe.EventKind
	}{
		{"acquire_succeeded", observe.EventAcquireSucceeded},
		{"contention", observe.EventContention},
		{"lease_lost", observe.EventLeaseLost},
		{"invalid", 0},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseEventKind(tt.input); got != tt.want {
				t.Errorf("ParseEventKind(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestClient_Health(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	got, err := c.Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got["status"] != "ok" {
		t.Fatalf("status = %q, want ok", got["status"])
	}
}

func TestClient_ErrorCases(t *testing.T) {
	t.Run("404", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()
		c := New(srv.URL)
		_, err := c.Snapshot(context.Background())
		if err == nil {
			t.Fatal("expected error for 404")
		}
	})
	t.Run("500", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}))
		defer srv.Close()
		c := New(srv.URL)
		_, err := c.Snapshot(context.Background())
		if err == nil {
			t.Fatal("expected error for 500")
		}
	})
	t.Run("connection refused", func(t *testing.T) {
		c := New("http://localhost:59999")
		_, err := c.Snapshot(context.Background())
		if err == nil {
			t.Fatal("expected error for connection refused")
		}
	})
}

func TestClient_Stream_SendsEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		w.Write([]byte(`data: {"Kind":2,"definition_id":"order","resource_id":"order:1","owner_id":"api-1","Timestamp":"2026-04-08T10:00:00Z"}
`))
		flusher.Flush()
	}))
	defer srv.Close()

	c := New(srv.URL)
	eventCh, errCh := c.Stream(context.Background())

	var got observe.Event
	select {
	case got = <-eventCh:
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	if got.Kind != observe.EventAcquireSucceeded {
		t.Errorf("kind = %v, want %v", got.Kind, observe.EventAcquireSucceeded)
	}

	for range eventCh {
	}
	for range errCh {
	}
}

func TestClient_Stream_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := New(srv.URL)
	eventCh, errCh := c.Stream(ctx)

	time.AfterFunc(50*time.Millisecond, cancel)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range eventCh {
		}
	}()
	go func() {
		defer wg.Done()
		for range errCh {
		}
	}()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("channels not closed after cancel")
	}
}
