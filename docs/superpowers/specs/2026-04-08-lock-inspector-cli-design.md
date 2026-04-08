# Distributed Lock Inspector CLI — Design Spec

## Overview

Full TUI application for inspecting lockman distributed locks via HTTP inspect endpoints. Written in Go with bubbletea + cobra.

## Repository Placement

The CLI lives as a new module under `cmd/inspect/` with its own `go.mod`:

```
cmd/inspect/
├── go.mod                     # module github.com/tuanuet/lockman/cmd/inspect
├── main.go                    # Entry point: calls cmd.Execute()
├── cmd/
│   └── root.go                # Cobra root: flags, subcommands, TUI launch
├── client/
│   └── client.go              # HTTP client wrapper
├── tui/
│   ├── app.go                 # Root model
│   ├── screens/               # TUI screens (dashboard, active, events, stream, health)
│   └── components/            # Shared components (table, tabbar, statusbar, filter)
└── sse/
    └── parse.go               # SSE frame parser (newline-delimited "data: " → JSON)
```

**Workspace changes:**
- Add `./cmd/inspect` to `go.work`
- `go get` dependencies: cobra, bubbletea, bubbles, lipgloss
- CLI imports `github.com/tuanuet/lockman/inspect` and `github.com/tuanuet/lockman/observe` from parent module (resolved via `go.work`)

**Binary name:** Built as `lockman-inspect` (e.g. `go build -o lockman-inspect ./cmd/inspect`)
**Cobra root command:** `Use: "lockman-inspect"` — matches binary name for consistent help text
**Version:** `cmd/inspect/v0.1.0` (independent from root SDK)

## Architecture

```
inspect-cli/
├── cmd/root.go              # Entry point: cobra, parse flags/env
├── client/
│   └── client.go            # HTTP client wrapper (typed API calls)
└── tui/
    ├── app.go               # Root model: navigation + lifecycle
    ├── screens/
    │   ├── dashboard.go     # Snapshot overview
    │   ├── active.go        # Active locks table
    │   ├── events.go        # Events list + filter modal
    │   ├── stream.go        # Real-time SSE feed
    │   └── health.go        # Health + pipeline stats
    └── components/
        ├── table.go         # Reusable table with scroll
        ├── tabbar.go        # Top navigation bar
        ├── statusbar.go     # Bottom keyboard hints
        └── filter.go        # Filter input modal
```

### Root Model
```go
type App struct {
    client     *client.Client
    screens    []tea.Model
    activeIdx  int
    errToast   string
    width      int
    height     int
}
```

**State ownership:**
- **Root owns:** active screen index, client, window dimensions, error toast
- **Each screen owns:** its data, loading state, scroll position, local filters
- **Shared via messages:** Screens send `tea.Cmd` to root for refresh. Root sends `ScreenRefreshMsg` to active screen. Modal state lives in the screen that opened it.

**Screen lifecycle:**
- `Init()` → returns `tea.Cmd` to fetch initial data
- `Update()` → handles local keys + global `tea.WindowSizeMsg`, `tea.KeyMsg{Type: tea.KeyTab}`
- `View()` → renders screen content (no tabbar/statusbar — those are root's job)
- Root wraps screen `View()` with tabbar (top) and statusbar (bottom)

## Client

```go
type Client struct {
    baseURL    string
    httpClient *http.Client
}

func (c *Client) Snapshot(ctx context.Context) (inspect.Snapshot, error)
func (c *Client) Active(ctx context.Context) ([]inspect.RuntimeLockInfo, error)
func (c *Client) Events(ctx context.Context, filter Filter) ([]observe.Event, error)
func (c *Client) Stream(ctx context.Context) (<-chan observe.Event, <-chan error)
func (c *Client) Health(ctx context.Context) (map[string]string, error)
```

### Filter
```go
type Filter struct {
    DefinitionID string
    ResourceID   string
    OwnerID      string
    Kind         observe.EventKind  // maps to inspect.QueryOptions.Kind; 0 = all
    Since        time.Time
    Until        time.Time
    Limit        int  // default 100, max 500
}
```

### Endpoint Mappings

| Method | Endpoint | Request | Response |
|--------|----------|---------|----------|
| `Snapshot` | `GET /locks/inspect` | — | `inspect.Snapshot` |
| `Active` | `GET /locks/inspect/active` | — | `[]inspect.RuntimeLockInfo` |
| `Events` | `GET /locks/inspect/events` | `?definition_id=&resource_id=&owner_id=&kind=&since=&until=&limit=` | `[]observe.Event` |
| `Stream` | `GET /locks/inspect/stream` | — | SSE stream of `observe.Event` |
| `Health` | `GET /locks/inspect/health` | — | `map[string]string{"status":"ok"}` |

**`observe.Event` fields displayed in CLI:**
- `Timestamp` (formatted as `15:04:05`)
- `Kind` (color-coded label)
- `DefinitionID`
- `ResourceID`
- `OwnerID`
- `Error` (shown inline if non-empty, e.g. for `acquire_failed`)

**`Kind` string mapping for filter input:**
Users type lowercase strings → parsed by CLI-local helper `parseEventKind(s)`:

| Input | `observe.EventKind` |
|-------|---------------------|
| `acquire_started` | `EventAcquireStarted` |
| `acquire_succeeded` | `EventAcquireSucceeded` |
| `acquire_failed` | `EventAcquireFailed` |
| `released` | `EventReleased` |
| `contention` | `EventContention` |
| `overlap` | `EventOverlap` |
| `overlap_rejected` | `EventOverlapRejected` |
| `lease_lost` | `EventLeaseLost` |
| `renewal_succeeded` | `EventRenewalSucceeded` |
| `renewal_failed` | `EventRenewalFailed` |
| `shutdown_started` | `EventShutdownStarted` |
| `shutdown_completed` | `EventShutdownCompleted` |
| `client_started` | `EventClientStarted` |
| `presence_checked` | `EventPresenceChecked` |
| (invalid/empty) | `0` (no filter) |

### Connection
- Flag: `--url` or env `LOCKMAN_INSPECT_URL`
- Default: `http://localhost:8080/locks/inspect`
- Stream: Parse SSE frames manually (`data: ` prefix → JSON decode)

### Stream Lifecycle

`Stream(ctx)` opens an SSE connection and returns two channels:
- `events` — receives `observe.Event` as they arrive
- `errors` — receives connection/parse errors (does NOT include `context.Canceled`)

**Closure behavior:**
- When `ctx` is cancelled → HTTP request cancelled → goroutine exits → both channels closed
- Caller must drain both channels until closed before starting a new stream
- Reconnect: caller creates a new context and calls `Stream()` again (client does NOT auto-reconnect internally; reconnect logic lives in the TUI screen model)

**Reconnect flow (in TUI stream screen):**
1. Stream errors → screen shows toast
2. Wait 2s → create new context → call `Stream()` again
3. Track retry count; after 3 failures → stop, show "Press R to reconnect"

## TUI Screens

### Dashboard (snapshot)
- 3-column layout: Active Locks | Pending Claims | Renewals
- Bottom row: Pipeline stats (dropped, failures) + Shutdown status
- Refresh: `R` key or auto-refresh every 5s

### Active
- Table: Definition | Resource | Owner | Acquired At | Duration
- Sortable by Owner/Definition/Acquired via `S`
- Row highlight: `↑/↓` navigate, `Enter` → lock details modal

### Events
- List: `[timestamp] kind definition resource`
- Color-coded by kind (success=green, failed=red, contention=yellow)
- `F` → filter modal (definition_id, resource_id, owner_id, kind)
- Pagination: `PgUp`/`PgDn` or scroll via mouse

### Stream
- Auto-scrolling log feed (SSE)
- Persistent filter bar at top
- `Space` pause/resume, `/` inline filter

### Health
- Status indicator from `/locks/inspect/health` (`{"status":"ok"}`)
- Pipeline stats fetched from `/locks/inspect` Snapshot (buffer size, dropped count, failures)
- Shutdown status from Snapshot
- Refresh: `R` key fetches both endpoints

## Components
- **TabBar**: Top row, highlights active screen, responsive width
- **StatusBar**: Bottom row, context-sensitive key hints
- **Table**: Scrollable, selectable rows, header sorting
- **FilterModal**: Text input popup, apply/cancel

## Error Handling

### Error Taxonomy
| Error Type | TUI Behavior | Subcommand Behavior |
|------------|-------------|---------------------|
| Connection refused / timeout | Toast: "Cannot reach <url>" + retry hint | Exit 1, stderr message |
| HTTP 404 | Inline: "Inspect endpoint not found — check --url" | Exit 1, stderr message |
| HTTP 500 | Inline: "Server error: <body>" | Exit 1, stderr message |
| JSON decode failure | Toast: "Invalid response from server" | Exit 1, stderr message |
| SSE parse error | Toast: "Stream parse error" + reconnect | N/A (subcommands don't stream) |
| Context cancelled | Graceful shutdown | Exit 130 (SIGINT) |

### Retry Policy
- **TUI screens (non-stream):** No auto-retry. User presses `R` to retry manually.
- **Stream:** Auto-reconnect on disconnect with exponential backoff: 2s, 4s, 8s. After 3 failed attempts, show toast: "Stream disconnected — press R to reconnect".
- **`Esc`** dismisses toasts, closes modals, and navigates back from detail views.

## Global Key Bindings
| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Next/prev screen |
| `1-5` | Jump to screen |
| `Ctrl+C` | Quit |
| `R` | Refresh current screen |
| `Esc` | Dismiss modal/error/back |

## Screen-Specific Keys
| Key | Screen | Action |
|-----|--------|--------|
| `↑/↓` | Active, Events | Navigate rows |
| `Enter` | Active | View lock details |
| `F` | Events | Open filter modal |
| `S` | Active | Toggle sort |
| `Space` | Stream | Pause/resume |
| `PgUp`/`PgDn` | Events | Page up/down |
| `/` | Stream | Inline filter |

## Color Palette
| Color | Usage |
|-------|-------|
| `#8be9fd` (cyan) | Headers, active tab |
| `#50fa7b` (green) | Success states |
| `#ff5555` (red) | Errors, failed events |
| `#f1fa8c` (yellow) | Warnings, contention |
| `#6272a4` (gray) | Dimmed text, timestamps |

## Dependencies

### Go Modules
- Module path: `github.com/tuanuet/lockman/cmd/inspect`
- New `go.mod` in `cmd/inspect/` with `go 1.25` (matches workspace)
- Added to root `go.work`

### Third-party
| Package | Usage |
|---------|-------|
| `github.com/spf13/cobra` | CLI entry, flags, subcommands |
| `github.com/charmbracelet/bubbletea` | TUI framework (Elm architecture) |
| `github.com/charmbracelet/bubbles/*` | viewport, list, textinput, table, spinner |
| `github.com/charmbracelet/lipgloss` | Styling, layout composition |

### Internal (from parent module via go.work)
| Package | Usage |
|---------|-------|
| `github.com/tuanuet/lockman/inspect` | `Snapshot`, `RuntimeLockInfo`, `WorkerClaimInfo`, `RenewalInfo`, `PipelineState` |
| `github.com/tuanuet/lockman/observe` | `Event`, `EventKind` constants |

### Standard Library
- `net/http` — HTTP client, SSE connection
- `encoding/json` — Response decoding
- `bufio` — SSE frame parsing (line-by-line)
- `context` — Request cancellation
- `fmt`, `time`, `sort`, `strings` — Formatting, duration, sorting, parsing

## Commands

```bash
# Interactive TUI (default)
lockman-inspect --url http://localhost:8080/locks/inspect

# One-shot snapshot (optional non-interactive mode)
lockman-inspect snapshot --url ...

# One-shot active locks
lockman-inspect active --url ...

# One-shot events
lockman-inspect events --url ... --kind acquire_succeeded --limit 50

# One-shot health
lockman-inspect health --url ...
```

All commands share the same binary. Default behavior is full TUI. Subcommands output raw text for scripting.

## Test Plan

### Client Tests (`client/`)
- `TestClient_Snapshot` — mock HTTP server returns valid JSON → decoded Snapshot
- `TestClient_Active` — empty array vs populated array
- `TestClient_Events` — filter params serialized correctly in query string
- `TestClient_Events_KindMapping` — string "contention" → `observe.EventContention`
- `TestClient_Stream` — SSE frame parsing: multi-line data, malformed lines
- `TestClient_Stream_ContextCancel` — context cancel closes both channels, goroutine exits
- `TestClient_Stream_ChannelDrain` — after cancel, both channels are closed and safe to drain
- `TestClient_Health` — returns map with "status":"ok"
- `TestClient_ErrorCases` — 404, 500, connection refused, JSON decode failure

### TUI Tests (`tui/`)
- `TestApp_Init` — all 5 screens initialized, activeIdx=0
- `TestApp_Navigation` — Tab/Shift+Tab cycles 0→1→2→3→4→0, 1-5 keys jump directly
- `TestApp_ScreenRefreshMsg` — root dispatches to correct screen
- `TestDashboard_View` — renders 3 columns with data
- `TestActive_Sort` — toggle sort changes row order
- `TestEvents_Filter` — filter modal applies, results match
- `TestStream_PauseResume` — Space pauses event display, resumes on second Space
- `TestStream_Reconnect` — disconnect triggers 3 retry attempts with backoff (2s, 4s, 8s), then stops with "Press R to reconnect"
- `TestStream_Reconnect` — disconnect triggers 3 retry attempts with backoff

### Integration Tests
- `TestEndToEnd_TUI` — start mock inspect server, launch TUI in headless mode, verify screen renders
- `TestSubcommands_Output` — `snapshot`, `active`, `events`, `health` subcommands produce valid JSON/text output

### CI
- `go test ./cmd/inspect/...` in CI workflow
- Compile check: `go build ./cmd/inspect`
