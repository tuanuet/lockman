# Lock Inspector CLI Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a full TUI CLI for inspecting lockman distributed locks via HTTP endpoints.

**Architecture:** Go module under `cmd/inspect/` with cobra entry, bubbletea TUI framework, HTTP client wrapper, 5 interactive screens (dashboard, active, events, stream, health), reusable components.

**Tech Stack:** Go 1.25, cobra, bubbletea, bubbles, lipgloss, lockman/inspect, lockman/observe

---

## Chunk 1: Module Setup + HTTP Client + SSE

### Task 1.1: Initialize CLI module and workspace

**Files:**
- Create: `cmd/inspect/go.mod`
- Modify: `go.work`
- Create: `cmd/inspect/main.go`

- [ ] **Step 1: Create go.mod**

```go
module github.com/tuanuet/lockman/cmd/inspect

go 1.25

require (
	github.com/charmbracelet/bubbles v0.21.0
	github.com/charmbracelet/bubbletea v1.3.10
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/spf13/cobra v1.9.1
	github.com/tuanuet/lockman v1.0.0
)
```

- [ ] **Step 2: Add to go.work**

Add `./cmd/inspect` to the `use` block in `go.work`.

- [ ] **Step 3: Run go work sync**

```bash
go work sync
cd cmd/inspect && go mod tidy
```

Expected: `go.mod` updated with resolved dependencies.

- [ ] **Step 4: Create main.go**

```go
package main

import "github.com/tuanuet/lockman/cmd/inspect/cmd"

func main() {
	cmd.Execute()
}
```

- [ ] **Step 5: Verify build placeholder**

```bash
go build -o /dev/null ./cmd/inspect 2>&1 || true
```

Expected: Compilation error (cmd package doesn't exist yet).

- [ ] **Step 6: Commit**

```bash
git add cmd/inspect/go.mod cmd/inspect/main.go go.work
git commit -m "feat: add inspect CLI module scaffolding"
```

### Task 1.2: HTTP Client + Stream method + Filter types

**Files:**
- Create: `cmd/inspect/client/client.go`
- Create: `cmd/inspect/client/client_test.go`

- [ ] **Step 1: Write client types, constructor, and all methods**

Full `client.go`:

```go
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 1024*64), 1024*64)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var evt observe.Event
			if err := json.Unmarshal([]byte(line[6:]), &evt); err != nil {
				errCh <- fmt.Errorf("client: parse SSE event: %w", err)
				continue
			}
			select {
			case eventCh <- evt:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("client: stream read error: %w", err)
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
```

- [ ] **Step 2: Write client tests**

Full `client_test.go`:

```go
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
		// Send one event then close
		w.Write([]byte(`data: {"kind":"acquire_succeeded","definition_id":"order"}
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

	// Drain channels
	for range eventCh {
	}
	for range errCh {
	}
}

func TestClient_Stream_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hold connection open until client cancels
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := New(srv.URL)
	eventCh, errCh := c.Stream(ctx)

	// Cancel after a short delay
	time.AfterFunc(50*time.Millisecond, cancel)

	// Both channels should close
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); for range eventCh {} }()
	go func() { defer wg.Done(); for range errCh {} }()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("channels not closed after cancel")
	}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./cmd/inspect/client/... -v
```

Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add cmd/inspect/client/
git commit -m "feat: add HTTP client with Stream SSE, all endpoints, and tests"
```

## Chunk 2: TUI Root + Components

### Task 2.1: TUI App root model

**Files:**
- Create: `cmd/inspect/tui/app.go`
- Create: `cmd/inspect/tui/app_test.go`
- Create: `cmd/inspect/tui/messages.go`

- [ ] **Step 1: Write messages**

```go
package tui

type ScreenRefreshMsg struct{}

type ScreenSwitchMsg int

type ErrToastMsg string

type ClearToastMsg struct{}

func ScreenSwitchTo(idx int) ScreenSwitchMsg {
	return ScreenSwitchMsg(idx)
}

func NextScreen(current, total int) ScreenSwitchMsg {
	return ScreenSwitchMsg((current + 1) % total)
}

func PrevScreen(current, total int) ScreenSwitchMsg {
	idx := current - 1
	if idx < 0 {
		idx = total - 1
	}
	return ScreenSwitchMsg(idx)
}
```

- [ ] **Step 2: Write App model**

```go
package tui

import (
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/cmd/inspect/tui/components"
)

const screenCount = 5

var screenNames = []string{
	"Dashboard", "Active", "Events", "Stream", "Health",
}

type App struct {
	client    *client.Client
	screens   []tea.Model
	activeIdx int
	errToast  string
	width     int
	height    int
}

func NewApp(c *client.Client, screens []tea.Model) *App {
	return &App{
		client:  c,
		screens: screens,
	}
}

func (m *App) Init() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.screens))
	for i, s := range m.screens {
		cmds[i] = s.Init()
	}
	return tea.Batch(cmds...)
}

func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyTab:
			return m, m.sendScreenCmd(ScreenSwitchTo(NextScreen(m.activeIdx, screenCount)))
		case tea.KeyShiftTab:
			return m, m.sendScreenCmd(ScreenSwitchTo(PrevScreen(m.activeIdx, screenCount)))
		case tea.KeyEsc:
			if m.errToast != "" {
				m.errToast = ""
				return m, nil
			}
		}
		if msg.String() >= "1" && msg.String() <= "5" {
			idx := int(msg.String()[0] - '1')
			return m, m.sendScreenCmd(ScreenSwitchTo(idx))
		}
	case ScreenSwitchMsg:
		m.activeIdx = int(msg)
		return m, m.sendScreenCmd(ScreenRefreshMsg{})
	case ErrToastMsg:
		m.errToast = string(msg)
		return m, nil
	case ClearToastMsg:
		m.errToast = ""
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		wm := tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height}
		_, cmd := m.screens[m.activeIdx].Update(wm)
		return m, cmd
	}

	model, cmd := m.screens[m.activeIdx].Update(msg)
	m.screens[m.activeIdx] = model
	return m, cmd
}

func (m *App) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	tabbar := components.RenderTabBar(screenNames, m.activeIdx, m.width)
	statusbar := components.RenderStatusBar(m.screenHints(), m.width)
	content := m.screens[m.activeIdx].View()

	var body string
	if m.errToast != "" {
		body = lipgloss.JoinVertical(lipgloss.Left,
			content,
			components.ErrorStyle.Render(m.errToast),
		)
	} else {
		body = content
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		tabbar,
		body,
		statusbar,
	)
}

func (m *App) sendScreenCmd(msg tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return msg
	}
}

func (m *App) screenHints() []string {
	hints := [][]string{
		{"Tab/1-5", "Navigate"},
		{"R", "Refresh"},
		{"Esc", "Dismiss"},
		{"Ctrl+C", "Quit"},
	}
	if m.activeIdx == 1 {
		hints = append(hints, []string{"↑/↓", "Select"}, []string{"S", "Sort"})
	} else if m.activeIdx == 2 {
		hints = append(hints, []string{"F", "Filter"}, []string{"PgUp/Dn", "Page"})
	} else if m.activeIdx == 3 {
		hints = append(hints, []string{"Space", "Pause"}, []string{"R", "Reconnect"})
	}
	var out []string
	for _, h := range hints {
		out = append(out, components.Hint(h[0], h[1]))
	}
	return out
}
```

- [ ] **Step 3: Write App tests**

```go
package tui

import (
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/tuanuet/lockman/cmd/inspect/client"
)

type mockModel struct {
	initCalled bool
	updateMsgs []tea.Msg
}

func (m *mockModel) Init() tea.Cmd {
	m.initCalled = true
	return nil
}

func (m *mockModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.updateMsgs = append(m.updateMsgs, msg)
	return m, nil
}

func (m *mockModel) View() string {
	return "mock"
}

func TestApp_Init(t *testing.T) {
	screens := make([]tea.Model, screenCount)
	for i := range screens {
		screens[i] = &mockModel{}
	}
	app := NewApp(client.New("http://localhost"), screens)
	app.Init()

	for i, s := range screens {
		if !s.(*mockModel).initCalled {
			t.Errorf("screen %d Init() not called", i)
		}
	}
}

func TestApp_Navigation(t *testing.T) {
	screens := make([]tea.Model, screenCount)
	for i := range screens {
		screens[i] = &mockModel{}
	}
	app := NewApp(client.New("http://localhost"), screens)

	app.Update(tea.KeyMsg{Type: tea.KeyTab})
	if app.activeIdx != 1 {
		t.Errorf("after Tab, activeIdx = %d, want 1", app.activeIdx)
	}

	app.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if app.activeIdx != 0 {
		t.Errorf("after ShiftTab, activeIdx = %d, want 0", app.activeIdx)
	}

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if app.activeIdx != 2 {
		t.Errorf("after '3', activeIdx = %d, want 2", app.activeIdx)
	}
}

func TestApp_ScreenRefreshMsg(t *testing.T) {
	screens := make([]tea.Model, screenCount)
	for i := range screens {
		screens[i] = &mockModel{}
	}
	app := NewApp(client.New("http://localhost"), screens)
	app.Update(ScreenRefreshMsg{})

	m := screens[0].(*mockModel)
	if len(m.updateMsgs) == 0 {
		t.Fatal("expected screen to receive refresh message")
	}
	_, ok := m.updateMsgs[len(m.updateMsgs)-1].(ScreenRefreshMsg)
	if !ok {
		t.Errorf("expected ScreenRefreshMsg, got %T", m.updateMsgs[len(m.updateMsgs)-1])
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./cmd/inspect/tui/... -v -run 'TestApp'
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/inspect/tui/app.go cmd/inspect/tui/app_test.go cmd/inspect/tui/messages.go
git commit -m "feat: add TUI root model with navigation, tabbar, statusbar, and tests"
```

### Task 2.2: Components — styles, TabBar, StatusBar, FilterModal, Table

**Files:**
- Create: `cmd/inspect/tui/components/styles.go`
- Create: `cmd/inspect/tui/components/tabbar.go`
- Create: `cmd/inspect/tui/components/statusbar.go`
- Create: `cmd/inspect/tui/components/filter.go`
- Create: `cmd/inspect/tui/components/table.go`

- [ ] **Step 1: Write styles**

```go
package components

import "github.com/charmbracelet/lipgloss"

var (
	Cyan         = lipgloss.Color("#8be9fd")
	Green        = lipgloss.Color("#50fa7b")
	Red          = lipgloss.Color("#ff5555")
	Yellow       = lipgloss.Color("#f1fa8c")
	Gray         = lipgloss.Color("#6272a4")

	TitleStyle     = lipgloss.NewStyle().Foreground(Cyan).Bold(true)
	DimStyle       = lipgloss.NewStyle().Foreground(Gray)
	SuccessStyle   = lipgloss.NewStyle().Foreground(Green)
	ErrorStyle     = lipgloss.NewStyle().Foreground(Red)
	WarnStyle      = lipgloss.NewStyle().Foreground(Yellow)
)
```

- [ ] **Step 2: Write TabBar**

```go
package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	tabActiveStyle   = lipgloss.NewStyle().Foreground(Cyan).Bold(true)
	tabInactiveStyle = lipgloss.NewStyle().Foreground(Gray)
)

func RenderTabBar(screenNames []string, activeIdx int, width int) string {
	tabs := make([]string, len(screenNames))
	for i, name := range screenNames {
		label := name
		if i == activeIdx {
			label = "● " + name
			tabs[i] = tabActiveStyle.Render(label)
		} else {
			tabs[i] = tabInactiveStyle.Render(label)
		}
	}
	bar := strings.Join(tabs, "  ")
	return lipgloss.NewStyle().
		Width(width).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Render(bar)
}
```

- [ ] **Step 3: Write StatusBar**

```go
package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var statusBarStyle = lipgloss.NewStyle().
	Foreground(Gray).
	Background(lipgloss.Color("#282a36")).
	Padding(0, 1)

func RenderStatusBar(hints []string, width int) string {
	content := ""
	for i, h := range hints {
		if i > 0 {
			content += "  "
		}
		content += h
	}
	return statusBarStyle.Width(width).Render(content)
}

func Hint(key, action string) string {
	return fmt.Sprintf("%s %s", key, action)
}
```

- [ ] **Step 4: Write FilterModal**

```go
package components

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type FilterModal struct {
	visible   bool
	defInput  textinput.Model
	resInput  textinput.Model
	ownInput  textinput.Model
	kindInput textinput.Model
	focused   int
}

func NewFilterModal() FilterModal {
	ti := func(placeholder string) textinput.Model {
		m := textinput.New()
		m.Placeholder = placeholder
		m.CharLimit = 64
		return m
	}
	m := FilterModal{
		defInput:  ti("definition_id"),
		resInput:  ti("resource_id"),
		ownInput:  ti("owner_id"),
		kindInput: ti("kind (e.g. contention)"),
		focused:   0,
	}
	m.defInput.Focus()
	return m
}

func (m *FilterModal) Show() {
	m.visible = true
	m.defInput.Focus()
	m.focused = 0
}

func (m *FilterModal) Hide() {
	m.visible = false
}

func (m *FilterModal) Visible() bool {
	return m.visible
}

func (m *FilterModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !m.visible {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			m.Hide()
			return m, nil
		case tea.KeyTab, tea.KeyShiftTab:
			m.focused = (m.focused + 1) % 4
			inputs := []*textinput.Model{&m.defInput, &m.resInput, &m.ownInput, &m.kindInput}
			for i, inp := range inputs {
				if i == m.focused {
					inp.Focus()
				} else {
					inp.Blur()
				}
			}
		case tea.KeyEnter:
			m.Hide()
			return m, nil
		}
	}
	var cmd tea.Cmd
	inputs := []*textinput.Model{&m.defInput, &m.resInput, &m.ownInput, &m.kindInput}
	for i, inp := range inputs {
		if i == m.focused {
			*inp, cmd = inp.Update(msg)
		}
	}
	return m, cmd
}

func (m *FilterModal) View() string {
	if !m.visible {
		return ""
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		BorderForeground(Cyan)

	content := lipgloss.JoinVertical(lipgloss.Left,
		"Filter Events",
		"Definition: "+m.defInput.View(),
		"Resource:   "+m.resInput.View(),
		"Owner:      "+m.ownInput.View(),
		"Kind:       "+m.kindInput.View(),
		"Tab: next field | Enter: apply | Esc: cancel",
	)
	return style.Render(content)
}

func (m *FilterModal) DefinitionID() string { return m.defInput.Value() }
func (m *FilterModal) ResourceID() string   { return m.resInput.Value() }
func (m *FilterModal) OwnerID() string      { return m.ownInput.Value() }
func (m *FilterModal) Kind() string         { return m.kindInput.Value() }
```

- [ ] **Step 5: Write Table component**

```go
package components

import (
	"fmt"
	"strings"
)

// Column defines a table column.
type Column struct {
	Title string
	Width int
}

// Table renders a simple text table with aligned columns.
func Table(columns []Column, rows [][]string, selected int) string {
	if len(rows) == 0 {
		headers := make([]string, len(columns))
		for i, c := range columns {
			headers[i] = c.Title
		}
		return DimStyle.Render(strings.Join(headers, "  "))
	}

	var lines []string
	headerFmt := make([]string, len(columns))
	for i, c := range columns {
		headerFmt[i] = fmt.Sprintf("%%-%ds", c.Width)
	}
	headerLine := fmt.Sprintf(strings.Join(headerFmt, "  "), make([]any, len(columns))...)
	for i, c := range columns {
		headerLine = fmt.Sprintf(strings.Join(make([]string, i+1), fmt.Sprintf("%%-%ds  ", c.Width)), c.Title)
		if i == 0 {
			headerLine = fmt.Sprintf("%-*s", c.Width, c.Title)
		} else {
			headerLine = headerLine + fmt.Sprintf("  %-*s", c.Width, c.Title)
		}
	}
	lines = append(lines, TitleStyle.Render(headerLine))

	for i, row := range rows {
		prefix := " "
		if i == selected {
			prefix = "▸ "
		}
		var cells []string
		for j, c := range columns {
			val := ""
			if j < len(row) {
				val = row[j]
			}
			cells = append(cells, fmt.Sprintf("%-*s", c.Width, val))
		}
		lines = append(lines, prefix+strings.Join(cells, "  "))
	}

	return strings.Join(lines, "\n")
}
```

- [ ] **Step 6: Run build check**

```bash
go build ./cmd/inspect/...
```

Expected: Compiles successfully.

- [ ] **Step 7: Commit**

```bash
git add cmd/inspect/tui/components/
git commit -m "feat: add TUI components (styles, tabbar, statusbar, filter modal, table)"
```

## Chunk 3: Dashboard + Active Screens

### Task 3.1: Dashboard screen

**Files:**
- Create: `cmd/inspect/tui/screens/dashboard.go`
- Create: `cmd/inspect/tui/screens/dashboard_test.go`

- [ ] **Step 1: Write full dashboard.go**

```go
package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/cmd/inspect/tui"
	"github.com/tuanuet/lockman/cmd/inspect/tui/components"
	"github.com/tuanuet/lockman/inspect"
)

type Dashboard struct {
	client   *client.Client
	snapshot *inspect.Snapshot
	loading  bool
	err      string
	width    int
	height   int
}

func NewDashboard(c *client.Client) *Dashboard {
	return &Dashboard{client: c, loading: true}
}

func (m *Dashboard) Init() tea.Cmd {
	return m.refreshCmd()
}

func (m *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tui.ScreenRefreshMsg:
		m.loading = true
		m.err = ""
		return m, m.refreshCmd()
	case tea.KeyMsg:
		if msg.String() == "r" {
			m.loading = true
			m.err = ""
			return m, m.refreshCmd()
		}
	case snapshotMsg:
		m.snapshot = &msg.Snapshot
		m.loading = false
		m.err = ""
	case errMsg:
		m.err = msg.Error()
		m.loading = false
	}
	return m, nil
}

func (m *Dashboard) View() string {
	if m.loading {
		return "Loading..."
	}
	if m.err != "" {
		return components.ErrorStyle.Render("Error: " + m.err)
	}
	if m.snapshot == nil {
		return "No data"
	}
	s := m.snapshot

	left := renderLockList("Active Locks", s.RuntimeLocks, m.width/3)
	mid := renderClaimList("Pending Claims", s.WorkerClaims, m.width/3)
	right := renderRenewalList("Renewals", s.Renewals, m.width/3)

	bottom := fmt.Sprintf("Pipeline: dropped=%d sink_failures=%d exporter_failures=%d | Shutdown: started=%v completed=%v",
		s.Pipeline.DroppedCount, s.Pipeline.SinkFailureCount, s.Pipeline.ExporterFailureCount,
		s.Shutdown.Started, s.Shutdown.Completed)

	content := lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right)
	content = lipgloss.JoinVertical(lipgloss.Left, content, components.DimStyle.Render(bottom))

	return lipgloss.NewStyle().Height(m.height - 4).Render(content)
}

func (m *Dashboard) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		snap, err := m.client.Snapshot(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return snapshotMsg{Snapshot: snap}
	}
}

func renderLockList(title string, locks []inspect.RuntimeLockInfo, width int) string {
	style := lipgloss.NewStyle().Width(width).Padding(0, 1)
	header := components.TitleStyle.Render(title)
	lines := []string{header}
	for _, l := range locks {
		lines = append(lines, fmt.Sprintf("%s/%s by %s", l.DefinitionID, l.ResourceID, l.OwnerID))
	}
	if len(lines) == 1 {
		lines = append(lines, components.DimStyle.Render("(none)"))
	}
	return style.Render(strings.Join(lines, "\n"))
}

func renderClaimList(title string, claims []inspect.WorkerClaimInfo, width int) string {
	style := lipgloss.NewStyle().Width(width).Padding(0, 1)
	header := components.TitleStyle.Render(title)
	lines := []string{header}
	for _, c := range claims {
		lines = append(lines, fmt.Sprintf("%s/%s by %s", c.DefinitionID, c.ResourceID, c.OwnerID))
	}
	if len(lines) == 1 {
		lines = append(lines, components.DimStyle.Render("(none)"))
	}
	return style.Render(strings.Join(lines, "\n"))
}

func renderRenewalList(title string, renewals []inspect.RenewalInfo, width int) string {
	style := lipgloss.NewStyle().Width(width).Padding(0, 1)
	header := components.TitleStyle.Render(title)
	lines := []string{header}
	for _, r := range renewals {
		lines = append(lines, fmt.Sprintf("%s/%s renewed %s ago",
			r.DefinitionID, r.ResourceID, time.Since(r.LastRenewed).Round(time.Second)))
	}
	if len(lines) == 1 {
		lines = append(lines, components.DimStyle.Render("(none)"))
	}
	return style.Render(strings.Join(lines, "\n"))
}

type snapshotMsg struct{ inspect.Snapshot }
type errMsg struct{ error }

func (e errMsg) Error() string { return e.error.Error() }
```

- [ ] **Step 2: Write dashboard_test.go**

```go
package screens

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/inspect"
)

func TestDashboard_View(t *testing.T) {
	snap := inspect.Snapshot{
		RuntimeLocks: []inspect.RuntimeLockInfo{
			{DefinitionID: "order", ResourceID: "order:1", OwnerID: "api-1"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, snap)
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	d := NewDashboard(c)
	model, _ := d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	d = model.(*Dashboard)

	view := d.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	if !strings.Contains(view, "order:1") {
		t.Errorf("view should contain lock data, got: %s", view)
	}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./cmd/inspect/tui/screens/... -v -run TestDashboard
```

Expected: Test passes.

- [ ] **Step 4: Commit**

```bash
git add cmd/inspect/tui/screens/dashboard*.go
git commit -m "feat: add dashboard screen with 3-column layout"
```

### Task 3.2: Active locks screen

**Files:**
- Create: `cmd/inspect/tui/screens/active.go`
- Create: `cmd/inspect/tui/screens/active_test.go`

- [ ] **Step 1: Write full active.go**

```go
package screens

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/cmd/inspect/tui"
	"github.com/tuanuet/lockman/cmd/inspect/tui/components"
	"github.com/tuanuet/lockman/inspect"
)

type Active struct {
	client   *client.Client
	locks    []inspect.RuntimeLockInfo
	loading  bool
	err      string
	selected int
	sortBy   int // 0=definition, 1=owner, 2=acquired
	width    int
	height   int
}

func NewActive(c *client.Client) *Active {
	return &Active{client: c, loading: true}
}

func (m *Active) Init() tea.Cmd {
	return m.refreshCmd()
}

func (m *Active) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tui.ScreenRefreshMsg:
		m.loading = true
		return m, m.refreshCmd()
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			m.loading = true
			m.err = ""
			return m, m.refreshCmd()
		case "s":
			m.sortBy = (m.sortBy + 1) % 3
			m.sortLocks()
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.locks)-1 {
				m.selected++
			}
		}
	case activeLocksMsg:
		m.locks = msg.Locks
		m.sortLocks()
		m.loading = false
		m.err = ""
	case errMsg:
		m.err = msg.Error()
		m.loading = false
	}
	return m, nil
}

func (m *Active) View() string {
	if m.loading {
		return "Loading active locks..."
	}
	if m.err != "" {
		return components.ErrorStyle.Render("Error: " + m.err)
	}

	columns := []components.Column{
		{Title: "Definition", Width: 25},
		{Title: "Resource", Width: 25},
		{Title: "Owner", Width: 15},
		{Title: "Acquired At", Width: 25},
	}

	var rows [][]string
	for _, l := range m.locks {
		rows = append(rows, []string{
			l.DefinitionID,
			l.ResourceID,
			l.OwnerID,
			l.AcquiredAt.Format("15:04:05"),
		})
	}

	header := components.TitleStyle.Render("Active Locks (S to sort, ↑/↓ to navigate)")
	table := components.Table(columns, rows, m.selected)

	return lipgloss.NewStyle().Height(m.height - 4).Render(header + "\n" + table)
}

func (m *Active) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		locks, err := m.client.Active(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return activeLocksMsg{Locks: locks}
	}
}

func (m *Active) sortLocks() {
	sort.Slice(m.locks, func(i, j int) bool {
		switch m.sortBy {
		case 0:
			return m.locks[i].DefinitionID < m.locks[j].DefinitionID
		case 1:
			return m.locks[i].OwnerID < m.locks[j].OwnerID
		case 2:
			return m.locks[i].AcquiredAt.Before(m.locks[j].AcquiredAt)
		default:
			return false
		}
	})
}

type activeLocksMsg struct{ Locks []inspect.RuntimeLockInfo }
```

- [ ] **Step 2: Write active_test.go**

```go
package screens

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/inspect"
)

func TestActive_Sort(t *testing.T) {
	locks := []inspect.RuntimeLockInfo{
		{DefinitionID: "b", OwnerID: "y", AcquiredAt: time.Now().Add(-time.Hour)},
		{DefinitionID: "a", OwnerID: "z", AcquiredAt: time.Now()},
		{DefinitionID: "c", OwnerID: "x", AcquiredAt: time.Now().Add(-2 * time.Hour)},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, locks)
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	a := NewActive(c)
	model, _ := a.Update(activeLocksMsg{Locks: locks})
	a = model.(*Active)

	a.sortBy = 0
	a.sortLocks()
	if a.locks[0].DefinitionID != "a" {
		t.Errorf("expected first definition 'a', got %q", a.locks[0].DefinitionID)
	}

	a.sortBy = 1
	a.sortLocks()
	if a.locks[0].OwnerID != "x" {
		t.Errorf("expected first owner 'x', got %q", a.locks[0].OwnerID)
	}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./cmd/inspect/tui/screens/... -v -run TestActive
```

Expected: Test passes.

- [ ] **Step 4: Commit**

```bash
git add cmd/inspect/tui/screens/active*.go
git commit -m "feat: add active locks screen with sortable table component"
```

## Chunk 4: Events + Stream + Health Screens

### Task 4.1: Events screen

**Files:**
- Create: `cmd/inspect/tui/screens/events.go`

- [ ] **Step 1: Write full events.go**

```go
package screens

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/cmd/inspect/tui"
	"github.com/tuanuet/lockman/cmd/inspect/tui/components"
	"github.com/tuanuet/lockman/observe"
)

type Events struct {
	client   *client.Client
	events   []observe.Event
	loading  bool
	err      string
	filter   components.FilterModal
	page     int
	pageSize int
	width    int
	height   int
}

func NewEvents(c *client.Client) *Events {
	return &Events{
		client:   c,
		filter:   components.NewFilterModal(),
		pageSize: 50,
		loading:  true,
	}
}

func (m *Events) Init() tea.Cmd {
	return m.refreshCmd()
}

func (m *Events) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.filter.Visible() {
		model, cmd := m.filter.Update(msg)
		m.filter = model.(components.FilterModal)
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter {
			m.applyFilter()
			return m, m.refreshCmd()
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tui.ScreenRefreshMsg:
		m.loading = true
		return m, m.refreshCmd()
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			m.loading = true
			m.err = ""
			return m, m.refreshCmd()
		case "f":
			m.filter.Show()
			return m, nil
		case "pgup":
			if m.page > 0 {
				m.page--
			}
		case "pgdown":
			m.page++
		}
	case eventsMsg:
		m.events = msg.Events
		m.loading = false
		m.err = ""
	case errMsg:
		m.err = msg.Error()
		m.loading = false
	}
	return m, nil
}

func (m *Events) View() string {
	if m.filter.Visible() {
		return m.filter.View()
	}
	if m.loading {
		return "Loading events..."
	}
	if m.err != "" {
		return components.ErrorStyle.Render("Error: " + m.err)
	}

	start := m.page * m.pageSize
	end := start + m.pageSize
	if start >= len(m.events) {
		return components.DimStyle.Render("No more events")
	}
	if end > len(m.events) {
		end = len(m.events)
	}

	lines := []string{components.TitleStyle.Render("Events (F to filter, PgUp/PgDn to navigate)")}
	for _, e := range m.events[start:end] {
		color := components.DimStyle
		switch e.Kind {
		case observe.EventAcquireSucceeded, observe.EventRenewalSucceeded, observe.EventReleased:
			color = components.SuccessStyle
		case observe.EventAcquireFailed, observe.EventLeaseLost, observe.EventRenewalFailed:
			color = components.ErrorStyle
		case observe.EventContention, observe.EventOverlap:
			color = components.WarnStyle
		}
		kindLabel := color.Render(kindLabel(e.Kind))
		line := fmt.Sprintf("%s %-20s %-25s %-20s %s",
			e.Timestamp.Format("15:04:05"),
			kindLabel,
			e.DefinitionID,
			e.ResourceID,
			e.OwnerID,
		)
		lines = append(lines, line)
	}

	if len(m.events) == 0 {
		lines = append(lines, components.DimStyle.Render("  No events"))
	}

	pageInfo := fmt.Sprintf("Page %d (%d total)", m.page+1, (len(m.events)+m.pageSize-1)/m.pageSize)
	lines = append(lines, components.DimStyle.Render(pageInfo))

	return lipgloss.NewStyle().Height(m.height - 4).Render(strings.Join(lines, "\n"))
}

func (m *Events) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		filter := client.Filter{
			Limit: 500,
		}
		if m.filter.Visible() {
			filter.DefinitionID = m.filter.DefinitionID()
			filter.ResourceID = m.filter.ResourceID()
			filter.OwnerID = m.filter.OwnerID()
			filter.Kind = client.ParseEventKind(m.filter.Kind())
		}
		events, err := m.client.Events(context.Background(), filter)
		if err != nil {
			return errMsg{err}
		}
		return eventsMsg{Events: events}
	}
}

func (m *Events) applyFilter() {
	m.page = 0
}

func kindLabel(k observe.EventKind) string {
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
		return "unknown"
	}
}

type eventsMsg struct{ Events []observe.Event }
```

- [ ] **Step 2: Run build check**

```bash
go build ./cmd/inspect/...
```

Expected: Compiles successfully.

- [ ] **Step 3: Commit**

```bash
git add cmd/inspect/tui/screens/events.go
git commit -m "feat: add events screen with filter and pagination"
```

### Task 4.2: Stream screen

**Files:**
- Create: `cmd/inspect/tui/screens/stream.go`
- Create: `cmd/inspect/tui/screens/stream_test.go`

- [ ] **Step 1: Write full stream.go**

```go
package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/cmd/inspect/tui"
	"github.com/tuanuet/lockman/cmd/inspect/tui/components"
	"github.com/tuanuet/lockman/observe"
)

type Stream struct {
	client     *client.Client
	events     []observe.Event
	paused     bool
	loading    bool
	err        string
	retries    int
	maxRetries int
	ctx        context.Context
	cancel     context.CancelFunc
	program    *tea.Program
	width      int
	height     int
}

func NewStream(c *client.Client) *Stream {
	ctx, cancel := context.WithCancel(context.Background())
	return &Stream{
		client:     c,
		maxRetries: 3,
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (m *Stream) Init() tea.Cmd {
	return m.startStream()
}

func (m *Stream) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tui.ScreenRefreshMsg:
		m.retries = 0
		m.err = ""
		m.cancel()
		m.ctx, m.cancel = context.WithCancel(context.Background())
		return m, m.startStream()
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			m.retries = 0
			m.err = ""
			m.cancel()
			m.ctx, m.cancel = context.WithCancel(context.Background())
			return m, m.startStream()
		case " ":
			m.paused = !m.paused
			return m, nil
		}
	case streamEventMsg:
		if !m.paused {
			m.events = append(m.events, msg.Event)
			if len(m.events) > 1000 {
				m.events = m.events[len(m.events)-1000:]
			}
		}
		return m, nil
	case streamErr:
		m.retries++
		if m.retries > m.maxRetries {
			m.err = "Stream disconnected — press R to reconnect"
			return m, nil
		}
		// Exponential backoff: 2s, 4s, 8s
		backoff := time.Duration(1<<m.retries) * time.Second
		return m, tea.Tick(backoff, func(time.Time) tea.Msg {
			return tui.ScreenRefreshMsg{}
		})
	case errMsg:
		m.err = msg.Error()
	}
	return m, nil
}

func (m *Stream) View() string {
	if m.loading {
		return "Connecting to stream..."
	}
	if m.err != "" && len(m.events) == 0 {
		return components.ErrorStyle.Render(m.err)
	}

	header := components.TitleStyle.Render("Stream (Space: pause, R: reconnect)")
	lines := []string{header}

	start := 0
	if len(m.events) > m.height-4 {
		start = len(m.events) - (m.height - 4)
	}
	for _, e := range m.events[start:] {
		color := components.DimStyle
		switch e.Kind {
		case observe.EventAcquireSucceeded:
			color = components.SuccessStyle
		case observe.EventAcquireFailed:
			color = components.ErrorStyle
		case observe.EventContention:
			color = components.WarnStyle
		}
		lines = append(lines, color.Render(fmt.Sprintf("%s %s %s/%s %s",
			e.Timestamp.Format("15:04:05"),
			kindLabel(e.Kind),
			e.DefinitionID,
			e.ResourceID,
			e.OwnerID,
		)))
	}

	if m.paused {
		lines = append(lines, components.WarnStyle.Render("  [PAUSED]"))
	}

	if m.err != "" && len(m.events) > 0 {
		lines = append(lines, components.ErrorStyle.Render(m.err))
	}

	return lipgloss.NewStyle().Height(m.height - 4).Render(strings.Join(lines, "\n"))
}

func (m *Stream) startStream() tea.Cmd {
	return func() tea.Msg {
		m.loading = true
		eventCh, errCh := m.client.Stream(m.ctx)
		m.loading = false

		go func() {
			for {
				select {
				case evt, ok := <-eventCh:
					if !ok {
						return
					}
					if m.program != nil {
						m.program.Send(streamEventMsg{Event: evt})
					}
				case err, ok := <-errCh:
					if !ok {
						return
					}
					if m.program != nil {
						m.program.Send(streamErr{err})
					}
				case <-m.ctx.Done():
					return
				}
			}
		}()

		return nil
	}
}

// SetProgram gives the screen access to the tea.Program for sending messages.
func (m *Stream) SetProgram(p *tea.Program) {
	m.program = p
}

type streamEventMsg struct{ Event observe.Event }
type streamErr struct{ error }

func (e streamErr) Error() string { return e.error.Error() }
```

- [ ] **Step 2: Write stream_test.go**

```go
package screens

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/observe"
)

func TestStream_PauseResume(t *testing.T) {
	c := client.New("http://localhost")
	s := NewStream(c)

	if s.paused {
		t.Error("expected not paused")
	}

	s.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if !s.paused {
		t.Error("expected paused after Space")
	}

	s.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if s.paused {
		t.Error("expected not paused after second Space")
	}
}

func TestStream_Reconnect(t *testing.T) {
	c := client.New("http://localhost")
	s := NewStream(c)
	s.maxRetries = 3

	// Simulate 3 errors → after 3rd, retries=3, still <= maxRetries, no error yet
	for i := 0; i < 3; i++ {
		s.Update(streamErr{fmt.Errorf("connection lost")})
	}

	// 4th error should trigger the error message
	s.Update(streamErr{fmt.Errorf("connection lost")})

	if s.err == "" {
		t.Error("expected error message after max retries")
	}
	if !strings.Contains(s.err, "Press R to reconnect") {
		t.Errorf("unexpected error: %s", s.err)
	}
}

func TestStream_AppendEvents(t *testing.T) {
	c := client.New("http://localhost")
	s := NewStream(c)

	// Simulate receiving events
	for i := 0; i < 5; i++ {
		s.Update(streamEventMsg{Event: observe.Event{
			Kind:         observe.EventAcquireSucceeded,
			DefinitionID: "order",
			ResourceID:   fmt.Sprintf("order:%d", i),
		}})
	}

	if len(s.events) != 5 {
		t.Errorf("expected 5 events, got %d", len(s.events))
	}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./cmd/inspect/tui/screens/... -v -run 'TestStream'
```

Expected: Tests pass.

- [ ] **Step 4: Commit**

```bash
git add cmd/inspect/tui/screens/stream*.go
git commit -m "feat: add stream screen with SSE, pause, and exponential backoff reconnect"
```

### Task 4.3: Health screen

**Files:**
- Create: `cmd/inspect/tui/screens/health.go`

- [ ] **Step 1: Write full health.go**

```go
package screens

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/cmd/inspect/tui"
	"github.com/tuanuet/lockman/cmd/inspect/tui/components"
	"github.com/tuanuet/lockman/inspect"
)

type Health struct {
	client   *client.Client
	status   map[string]string
	snapshot *inspect.Snapshot
	loading  bool
	err      string
	width    int
	height   int
}

func NewHealth(c *client.Client) *Health {
	return &Health{client: c, loading: true}
}

func (m *Health) Init() tea.Cmd {
	return m.refreshCmd()
}

func (m *Health) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tui.ScreenRefreshMsg:
		m.loading = true
		m.err = ""
		return m, m.refreshCmd()
	case tea.KeyMsg:
		if msg.String() == "r" {
			m.loading = true
			m.err = ""
			return m, m.refreshCmd()
		}
	case healthMsg:
		m.status = msg.Status
		m.snapshot = msg.Snapshot
		m.loading = false
		m.err = ""
	case errMsg:
		m.err = msg.Error()
		m.loading = false
	}
	return m, nil
}

func (m *Health) View() string {
	if m.loading {
		return "Checking health..."
	}
	if m.err != "" {
		return components.ErrorStyle.Render("Error: " + m.err)
	}

	statusLabel := components.ErrorStyle.Render("Unhealthy")
	if m.status != nil && m.status["status"] == "ok" {
		statusLabel = components.SuccessStyle.Render("Healthy")
	}

	lines := []string{
		components.TitleStyle.Render("Health Status"),
		statusLabel,
		"",
		components.TitleStyle.Render("Pipeline Stats"),
	}

	if m.snapshot != nil {
		p := m.snapshot.Pipeline
		lines = append(lines,
			fmt.Sprintf("  Buffer size:     %d", p.BufferSize),
			fmt.Sprintf("  Dropped:         %d", p.DroppedCount),
			fmt.Sprintf("  Sink failures:   %d", p.SinkFailureCount),
			fmt.Sprintf("  Exporter fails:  %d", p.ExporterFailureCount),
			"",
			components.TitleStyle.Render("Shutdown"),
			fmt.Sprintf("  Started:   %v", m.snapshot.Shutdown.Started),
			fmt.Sprintf("  Completed: %v", m.snapshot.Shutdown.Completed),
		)
	}

	return lipgloss.NewStyle().Height(m.height - 4).Render(strings.Join(lines, "\n"))
}

func (m *Health) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		status, err := m.client.Health(ctx)
		if err != nil {
			return errMsg{err}
		}
		snap, err := m.client.Snapshot(ctx)
		if err != nil {
			snap = inspect.Snapshot{}
		}
		return healthMsg{Status: status, Snapshot: snap}
	}
}

type healthMsg struct {
	Status   map[string]string
	Snapshot inspect.Snapshot
}
```

- [ ] **Step 2: Run build check**

```bash
go build ./cmd/inspect/...
```

Expected: Compiles successfully.

- [ ] **Step 3: Commit**

```bash
git add cmd/inspect/tui/screens/health.go
git commit -m "feat: add health screen with pipeline stats"
```

## Chunk 5: Wire Everything + Subcommands

### Task 5.1: Wire root command

**Files:**
- Create: `cmd/inspect/cmd/root.go`

- [ ] **Step 1: Write full root.go**

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/cmd/inspect/tui"
	"github.com/tuanuet/lockman/cmd/inspect/tui/screens"
)

var (
	baseURL string
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "lockman-inspect",
	Short: "Interactive TUI for lockman distributed locks",
	Long: `Full TUI application for inspecting lockman distributed locks
via HTTP inspect endpoints.

Use subcommands (snapshot, active, events, health) for scripted output.`,
	RunE: runTUI,
}

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Print full snapshot as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(baseURL)
		snap, err := c.Snapshot(cmd.Context())
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(snap)
	},
}

var activeCmd = &cobra.Command{
	Use:   "active",
	Short: "Print active locks as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(baseURL)
		locks, err := c.Active(cmd.Context())
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(locks)
	},
}

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Print events as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(baseURL)
		kindStr, _ := cmd.Flags().GetString("kind")
		limit, _ := cmd.Flags().GetInt("limit")
		defID, _ := cmd.Flags().GetString("definition-id")
		resID, _ := cmd.Flags().GetString("resource-id")
		ownID, _ := cmd.Flags().GetString("owner-id")

		filter := client.Filter{
			DefinitionID: defID,
			ResourceID:   resID,
			OwnerID:      ownID,
			Kind:         client.ParseEventKind(kindStr),
			Limit:        limit,
		}
		events, err := c.Events(cmd.Context(), filter)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(events)
	},
}

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Print health status as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(baseURL)
		status, err := c.Health(cmd.Context())
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(status)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&baseURL, "url", "u",
		defaultURL(), "Inspect endpoint base URL")

	rootCmd.AddCommand(snapshotCmd)
	rootCmd.AddCommand(activeCmd)
	rootCmd.AddCommand(eventsCmd)
	rootCmd.AddCommand(healthCmd)

	eventsCmd.Flags().String("kind", "", "Filter by event kind (e.g. contention)")
	eventsCmd.Flags().Int("limit", 100, "Max events to return")
	eventsCmd.Flags().String("definition-id", "", "Filter by definition ID")
	eventsCmd.Flags().String("resource-id", "", "Filter by resource ID")
	eventsCmd.Flags().String("owner-id", "", "Filter by owner ID")
}

func defaultURL() string {
	if u := os.Getenv("LOCKMAN_INSPECT_URL"); u != "" {
		return u
	}
	return "http://localhost:8080/locks/inspect"
}

func runTUI(cmd *cobra.Command, args []string) error {
	c := client.New(baseURL)

	streamScreen := screens.NewStream(c)

	app := tui.NewApp(c, []tea.Model{
		screens.NewDashboard(c),
		screens.NewActive(c),
		screens.NewEvents(c),
		streamScreen,
		screens.NewHealth(c),
	})

	p := tea.NewProgram(app, tea.WithAltScreen())
	streamScreen.SetProgram(p)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Build and test TUI launch**

```bash
go build -o /tmp/lockman-inspect ./cmd/inspect
/tmp/lockman-inspect --help
```

Expected: Help text showing `--url` flag and subcommands.

- [ ] **Step 3: Test subcommand help**

```bash
/tmp/lockman-inspect events --help
```

Expected: Shows `--kind`, `--limit`, `--definition-id` flags.

- [ ] **Step 4: Commit**

```bash
git add cmd/inspect/cmd/root.go
git commit -m "feat: wire root command with TUI and all subcommands"
```

## Chunk 6: Final Integration + CI

### Task 6.1: Workspace sync + full build

**Files:**
- Modify: `go.work` (already done in Task 1.1)
- Auto: `go.work.sum`

- [ ] **Step 1: Ensure go.work includes cmd/inspect**

Verify `./cmd/inspect` is in the `use` block.

- [ ] **Step 2: Run go work sync and tidy**

```bash
go work sync
cd cmd/inspect && go mod tidy && cd ../..
```

- [ ] **Step 3: Build all**

```bash
go build ./cmd/inspect
```

Expected: Binary builds at `./lockman-inspect` or via `-o`.

- [ ] **Step 4: Run all CLI tests**

```bash
go test ./cmd/inspect/... -v
```

Expected: All tests pass.

- [ ] **Step 5: Verify existing repo tests still pass**

```bash
go test ./...
GOWORK=off go test ./...
```

Expected: Existing tests still pass.

- [ ] **Step 6: Commit**

```bash
git add go.work go.work.sum
git commit -m "chore: wire inspect CLI into workspace"
```

### Task 6.2: Integration tests + docs

**Files:**
- Create: `cmd/inspect/integration_test.go`
- Modify: `AGENTS.md`
- Create: `cmd/inspect/README.md`

- [ ] **Step 1: Write integration tests**

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/observe"
)

// writeJSON helper for test server
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func TestSubcommands_Output(t *testing.T) {
	snap := inspect.Snapshot{
		RuntimeLocks: []inspect.RuntimeLockInfo{
			{DefinitionID: "order", ResourceID: "order:1", OwnerID: "api-1", AcquiredAt: time.Now()},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/locks/inspect":
			writeJSON(w, snap)
		case "/locks/inspect/active":
			writeJSON(w, snap.RuntimeLocks)
		case "/locks/inspect/events":
			writeJSON(w, []observe.Event{})
		case "/locks/inspect/health":
			writeJSON(w, map[string]string{"status": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tests := []struct {
		subcommand string
		args       []string
		check      func(output string) error
	}{
		{
			subcommand: "snapshot",
			args:       []string{"--url", srv.URL + "/locks/inspect"},
			check: func(output string) error {
				if !strings.Contains(output, "order:1") {
					return fmt.Errorf("expected order:1 in output")
				}
				return nil
			},
		},
		{
			subcommand: "active",
			args:       []string{"--url", srv.URL + "/locks/inspect"},
			check: func(output string) error {
				if !strings.Contains(output, "api-1") {
					return fmt.Errorf("expected api-1 in output")
				}
				return nil
			},
		},
		{
			subcommand: "health",
			args:       []string{"--url", srv.URL + "/locks/inspect"},
			check: func(output string) error {
				if !strings.Contains(output, "ok") {
					return fmt.Errorf("expected ok in health output")
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.subcommand, func(t *testing.T) {
			args := append([]string{"run", "./cmd/inspect", tt.subcommand}, tt.args...)
			cmd := exec.Command("go", args...)
			cmd.Dir = "." // repo root
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("command failed: %v\n%s", err, out)
			}
			if err := tt.check(string(out)); err != nil {
				t.Errorf("output check: %v\noutput: %s", err, out)
			}
		})
	}
}
```

Note: the `writeJSON` function above needs `"fmt"` import — add it.

- [ ] **Step 2: Run integration tests**

```bash
go test ./cmd/inspect/... -v -run 'TestSubcommands'
```

Expected: Tests pass.

- [ ] **Step 3: Add to AGENTS.md**

Add under "Primary Commands" section:

```
### CLI Commands

- Build CLI:
  - `go build -o lockman-inspect ./cmd/inspect`
- Run CLI:
  - `go run ./cmd/inspect --url http://localhost:8080/locks/inspect`
- Test CLI:
  - `go test ./cmd/inspect/...`
```

- [ ] **Step 4: Create CLI README**

```markdown
# lockman-inspect

Interactive TUI for inspecting lockman distributed locks.

## Usage

```bash
# Interactive TUI
lockman-inspect --url http://localhost:8080/locks/inspect

# One-shot commands
lockman-inspect snapshot --url ...
lockman-inspect active --url ...
lockman-inspect events --url ... --kind contention
lockman-inspect health --url ...
```

## Environment Variables

- `LOCKMAN_INSPECT_URL` — default base URL
```

- [ ] **Step 5: Final commit**

```bash
git add cmd/inspect/integration_test.go AGENTS.md cmd/inspect/README.md
git commit -m "test: add integration tests, docs, AGENTS.md update"
```
