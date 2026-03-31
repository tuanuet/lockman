package inspect

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/tuanuet/lockman/observe"
)

// HandlerOption configures the HTTP handler.
type HandlerOption func(*handlerConfig)

type handlerConfig struct {
	prefix string
}

// WithPrefix sets a URL prefix for all inspect routes.
func WithPrefix(p string) HandlerOption {
	return func(c *handlerConfig) { c.prefix = p }
}

// NewHandler returns an http.Handler exposing inspect endpoints.
func NewHandler(store *Store, opts ...HandlerOption) http.Handler {
	cfg := &handlerConfig{prefix: "/locks/inspect"}
	for _, o := range opts {
		o(cfg)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.prefix, handleSnapshot(store))
	mux.HandleFunc(cfg.prefix+"/active", handleActive(store))
	mux.HandleFunc(cfg.prefix+"/events", handleEvents(store))
	mux.HandleFunc(cfg.prefix+"/health", handleHealth(store))
	mux.HandleFunc(cfg.prefix+"/stream", handleStream(store))

	return mux
}

func handleSnapshot(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, store.Snapshot())
	}
}

func handleActive(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		snap := store.Snapshot()
		writeJSON(w, snap.RuntimeLocks)
	}
}

func handleEvents(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		q := r.URL.Query()
		limit := 100
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}

		opts := QueryOptions{
			DefinitionID: q.Get("definition_id"),
			ResourceID:   q.Get("resource_id"),
			OwnerID:      q.Get("owner_id"),
		}

		if v := q.Get("since"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				opts.Since = t
			}
		}
		if v := q.Get("until"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				opts.Until = t
			}
		}
		if v := q.Get("kind"); v != "" {
			opts.Kind = parseKind(v)
		}

		events := store.Query(opts)
		if len(events) > limit {
			events = events[len(events)-limit:]
		}

		writeJSON(w, events)
	}
}

func handleHealth(_ *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func handleStream(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ch := make(chan observe.Event, 64)
		unsub := store.Subscribe(ch)
		defer unsub()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case e := <-ch:
				data, _ := json.Marshal(e)
				_, _ = w.Write([]byte("data: "))
				_, _ = w.Write(data)
				_, _ = w.Write([]byte("\n\n"))
				flusher.Flush()
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func parseKind(s string) observe.EventKind {
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
