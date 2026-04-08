# Distributed Lock Inspector CLI — Design Spec

## Overview

Full TUI application for inspecting lockman distributed locks via HTTP inspect endpoints. Written in Go with bubbletea + cobra.

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
- Holds 5 child screen models as `tea.Model`
- Active screen index (0-4), switchable via `Tab`/`Shift+Tab` or `1-5`
- Shared HTTP client instance
- Global error toast state
- Delegates `Update`/`View` to active screen

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
    Kind         string
    Since        time.Time
    Until        time.Time
    Limit        int
}
```

### Connection
- Flag: `--url` or env `LOCKMAN_INSPECT_URL`
- Default: `http://localhost:8080/locks/inspect`
- Stream: Parse SSE frames manually (`data: ` prefix → JSON decode)

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
- Pagination: `Space` next, `b` previous

### Stream
- Auto-scrolling log feed (SSE)
- Persistent filter bar at top
- `Space` pause/resume, `/` inline filter

### Health
- Status indicator + JSON response view
- Pipeline stats: buffer size, dropped count, failures
- Refresh: `R` key

## Components
- **TabBar**: Top row, highlights active screen, responsive width
- **StatusBar**: Bottom row, context-sensitive key hints
- **Table**: Scrollable, selectable rows, header sorting
- **FilterModal**: Text input popup, apply/cancel

## Error Handling
- Network errors → bottom toast, app stays alive
- `404/500` → inline message in current screen
- Stream disconnect → auto-reconnect after 2s (max 3 attempts)
- `Esc` dismisses errors/modals

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
| `Space` | Stream, Events | Pause/resume |
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
- `github.com/spf13/cobra` — CLI entry, flags
- `github.com/charmbracelet/bubbletea` — TUI framework
- `github.com/charmbracelet/bubbles/*` — Input, list, table, viewport
- `github.com/charmbracelet/lipgloss` — Styling
- `github.com/tuanuet/lockman/inspect` — Types (Snapshot, RuntimeLockInfo)
- `github.com/tuanuet/lockman/observe` — Event types

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
