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
	return tea.Batch(
		m.refreshCmd(),
		tea.Tick(5*time.Second, func(time.Time) tea.Msg {
			return tui.ScreenRefreshMsg{}
		}),
	)
}

func (m *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tui.ScreenRefreshMsg:
		m.loading = true
		m.err = ""
		return m, tea.Batch(
			m.refreshCmd(),
			tea.Tick(5*time.Second, func(time.Time) tea.Msg {
				return tui.ScreenRefreshMsg{}
			}),
		)
	case tea.KeyMsg:
		if msg.String() == "r" {
			m.loading = true
			m.err = ""
			return m, tea.Batch(
				m.refreshCmd(),
				tea.Tick(5*time.Second, func(time.Time) tea.Msg {
					return tui.ScreenRefreshMsg{}
				}),
			)
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
