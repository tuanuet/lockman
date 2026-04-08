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

## Screens

- **Dashboard** — 3-column overview of active locks, pending claims, renewals
- **Active** — Sortable table of active locks with duration
- **Events** — Filtered, paginated event history
- **Stream** — Real-time SSE event feed with pause and filter
- **Health** — Service health status and pipeline stats

## Navigation

- `Tab` / `1-5` — Switch screens
- `R` — Refresh current screen
- `Esc` — Dismiss errors/modals
- `Ctrl+C` — Quit
