package inspect

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tuanuet/lockman/observe"
)

type Store interface {
	Consume(ctx context.Context, event observe.Event) error
	Snapshot() Snapshot
	RecentEvents() []Event
	Query(filter QueryFilter) []Event
	UpdatePipelineState(state PipelineState)
	Events() <-chan Event
}

type HandlerOption func(*handler)

type handler struct {
	store Store
}

func NewHandler(store Store, opts ...HandlerOption) http.Handler {
	h := &handler{store: store}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case path == "/locks/inspect" || path == "/locks/inspect/":
		h.handleSnapshot(w, r)
	case path == "/locks/inspect/active":
		h.handleActive(w, r)
	case path == "/locks/inspect/events":
		h.handleEvents(w, r)
	case path == "/locks/inspect/health":
		h.handleHealth(w, r)
	case path == "/locks/inspect/stream":
		h.handleStream(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *handler) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	snap := h.store.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snap)
}

func (h *handler) handleActive(w http.ResponseWriter, r *http.Request) {
	snap := h.store.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"runtime_locks": snap.RuntimeLocks,
		"worker_claims": snap.WorkerClaims,
	})
}

func (h *handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	filter := parseQueryFilter(r.URL.Query())
	events := h.store.Query(filter)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func (h *handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	snap := h.store.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      snap.Pipeline.Status,
		"queue_depth": snap.Pipeline.QueueDepth,
		"errors":      snap.Pipeline.Errors,
	})
}

func (h *handler) handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	events := h.store.Events()
	for {
		select {
		case e, ok := <-events:
			if !ok {
				return
			}
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		case <-time.After(30 * time.Second):
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func parseQueryFilter(params map[string][]string) QueryFilter {
	var filter QueryFilter

	if v, ok := params["lock_id"]; ok && len(v) > 0 {
		filter.LockID = v[0]
	}
	if v, ok := params["resource_key"]; ok && len(v) > 0 {
		filter.ResourceKey = v[0]
	}
	if v, ok := params["owner_id"]; ok && len(v) > 0 {
		filter.OwnerID = v[0]
	}
	if v, ok := params["kind"]; ok && len(v) > 0 {
		filter.Kind = stringToEventKind(v[0])
	}
	if v, ok := params["since"]; ok && len(v) > 0 {
		if t, err := time.Parse(time.RFC3339, v[0]); err == nil {
			filter.Since = t
		}
	}
	if v, ok := params["until"]; ok && len(v) > 0 {
		if t, err := time.Parse(time.RFC3339, v[0]); err == nil {
			filter.Until = t
		}
	}

	return filter
}

func stringToEventKind(s string) observe.EventKind {
	switch s {
	case "acquire_started":
		return observe.EventAcquireStarted
	case "acquire_succeeded":
		return observe.EventAcquireSucceeded
	case "acquire_failed":
		return observe.EventAcquireFailed
	case "released":
		return observe.EventReleased
	case "contention":
		return observe.EventContention
	case "overlap":
		return observe.EventOverlap
	case "lease_lost":
		return observe.EventLeaseLost
	case "renewal_succeeded":
		return observe.EventRenewalSucceeded
	case "renewal_failed":
		return observe.EventRenewalFailed
	case "shutdown_started":
		return observe.EventShutdownStarted
	case "shutdown_completed":
		return observe.EventShutdownCompleted
	default:
		return 0
	}
}
