package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/tuanuet/lockman/cmd/inspect/sse"
	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/observe"
)

const (
	defaultTimeout = 10 * time.Second
	defaultLimit   = 100
	maxLimit       = 500
)

type Filter struct {
	DefinitionID string
	ResourceID   string
	OwnerID      string
	Kind         observe.EventKind
	Since        time.Time
	Until        time.Time
	Limit        int
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
	}
}

func (c *Client) Snapshot(ctx context.Context) (inspect.Snapshot, error) {
	var snap inspect.Snapshot
	if err := c.doJSON(ctx, "", &snap); err != nil {
		return inspect.Snapshot{}, err
	}
	return snap, nil
}

func (c *Client) Active(ctx context.Context) ([]inspect.RuntimeLockInfo, error) {
	var locks []inspect.RuntimeLockInfo
	if err := c.doJSON(ctx, "/active", &locks); err != nil {
		return nil, err
	}
	return locks, nil
}

func (c *Client) Events(ctx context.Context, filter Filter) ([]observe.Event, error) {
	q := make(url.Values)
	if filter.DefinitionID != "" {
		q.Set("definition_id", filter.DefinitionID)
	}
	if filter.ResourceID != "" {
		q.Set("resource_id", filter.ResourceID)
	}
	if filter.OwnerID != "" {
		q.Set("owner_id", filter.OwnerID)
	}
	if filter.Kind != 0 {
		q.Set("kind", kindToString(filter.Kind))
	}
	if !filter.Since.IsZero() {
		q.Set("since", filter.Since.Format(time.RFC3339))
	}
	if !filter.Until.IsZero() {
		q.Set("until", filter.Until.Format(time.RFC3339))
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	q.Set("limit", strconv.Itoa(limit))

	var events []observe.Event
	if err := c.doJSON(ctx, "/events?"+q.Encode(), &events); err != nil {
		return nil, err
	}
	return events, nil
}

func (c *Client) Health(ctx context.Context) (map[string]string, error) {
	var status map[string]string
	if err := c.doJSON(ctx, "/health", &status); err != nil {
		return nil, err
	}
	return status, nil
}

// Stream opens an SSE connection and returns two channels.
// events receives observe.Event as they arrive.
// errors receives connection/parse errors (not context.Canceled).
// Both channels are closed when the stream ends or ctx is cancelled.
func (c *Client) Stream(ctx context.Context) (<-chan observe.Event, <-chan error) {
	eventCh := make(chan observe.Event, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/stream", nil)
		if err != nil {
			errCh <- fmt.Errorf("client: build stream request: %w", err)
			return
		}
		req.Header.Set("Accept", "text/event-stream")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			errCh <- fmt.Errorf("client: stream request failed: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("client: stream returned %d: %s", resp.StatusCode, bytes.TrimSpace(body))
			return
		}

		rawCh := make(chan json.RawMessage, 64)
		rawErrCh := make(chan error, 1)
		go sse.ParseEvents(resp.Body, rawCh, rawErrCh)

		for {
			select {
			case raw, ok := <-rawCh:
				if !ok {
					return
				}
				var evt observe.Event
				if err := json.Unmarshal(raw, &evt); err != nil {
					errCh <- fmt.Errorf("client: parse event: %w", err)
					continue
				}
				select {
				case eventCh <- evt:
				case <-ctx.Done():
					return
				}
			case err, ok := <-rawErrCh:
				if !ok {
					return
				}
				errCh <- fmt.Errorf("client: SSE parse: %w", err)
			case <-ctx.Done():
				return
			}
		}
	}()

	return eventCh, errCh
}

func (c *Client) doJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("client: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("client: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("client: server returned %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("client: decode response: %w", err)
	}
	return nil
}

func ParseEventKind(s string) observe.EventKind {
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
	case "overlap_rejected":
		return observe.EventOverlapRejected
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
	case "client_started":
		return observe.EventClientStarted
	case "presence_checked":
		return observe.EventPresenceChecked
	default:
		return 0
	}
}

func kindToString(k observe.EventKind) string {
	switch k {
	case observe.EventAcquireStarted:
		return "acquire_started"
	case observe.EventAcquireSucceeded:
		return "acquire_succeeded"
	case observe.EventAcquireFailed:
		return "acquire_failed"
	case observe.EventReleased:
		return "released"
	case observe.EventContention:
		return "contention"
	case observe.EventOverlap:
		return "overlap"
	case observe.EventOverlapRejected:
		return "overlap_rejected"
	case observe.EventLeaseLost:
		return "lease_lost"
	case observe.EventRenewalSucceeded:
		return "renewal_succeeded"
	case observe.EventRenewalFailed:
		return "renewal_failed"
	case observe.EventShutdownStarted:
		return "shutdown_started"
	case observe.EventShutdownCompleted:
		return "shutdown_completed"
	case observe.EventClientStarted:
		return "client_started"
	case observe.EventPresenceChecked:
		return "presence_checked"
	default:
		return ""
	}
}
