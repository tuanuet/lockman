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
		s := msg.Snapshot
		m.snapshot = &s
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

	s := m.snapshot
	activeCount := len(s.RuntimeLocks)
	claimCount := len(s.WorkerClaims)
	renewalCount := len(s.Renewals)

	lines := []string{
		components.TitleStyle.Render("Health Status"),
		statusLabel,
		"",
		components.TitleStyle.Render("Active Locks"),
		components.RowStyle.Render(fmt.Sprintf("  Locks held:     %d", activeCount)),
		components.RowStyle.Render(fmt.Sprintf("  Pending claims: %d", claimCount)),
		components.RowStyle.Render(fmt.Sprintf("  Renewals:       %d", renewalCount)),
	}

	if activeCount > 0 {
		lines = append(lines, "")
		lines = append(lines, components.TitleStyle.Render("Top Active Locks"))
		limit := activeCount
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			l := s.RuntimeLocks[i]
			label := fmt.Sprintf("  %s/%s by %s", l.DefinitionID, l.ResourceID, l.OwnerID)
			lines = append(lines, components.RowStyle.Render(label))
		}
	}

	if m.snapshot != nil && m.snapshot.Pipeline.DroppedCount > 0 {
		p := m.snapshot.Pipeline
		lines = append(lines, "")
		lines = append(lines, components.TitleStyle.Render("Pipeline Stats"))
		lines = append(lines,
			components.RowStyle.Render(fmt.Sprintf("  Dropped:         %d", p.DroppedCount)),
			components.RowStyle.Render(fmt.Sprintf("  Sink failures:   %d", p.SinkFailureCount)),
			components.RowStyle.Render(fmt.Sprintf("  Exporter fails:  %d", p.ExporterFailureCount)),
		)
	}

	if m.snapshot != nil {
		lines = append(lines, "")
		lines = append(lines, components.TitleStyle.Render("Shutdown"))
		started := components.SuccessStyle.Render(fmt.Sprintf("  Started:   %v", m.snapshot.Shutdown.Started))
		completed := components.SuccessStyle.Render(fmt.Sprintf("  Completed: %v", m.snapshot.Shutdown.Completed))
		if m.snapshot.Shutdown.Started {
			started = components.WarnStyle.Render(fmt.Sprintf("  Started:   %v", m.snapshot.Shutdown.Started))
		}
		if m.snapshot.Shutdown.Completed {
			completed = components.ErrorStyle.Render(fmt.Sprintf("  Completed: %v", m.snapshot.Shutdown.Completed))
		}
		lines = append(lines, started, completed)
	}

	content := strings.Join(lines, "\n")
	if m.height == 0 {
		return content
	}

	h := m.height - 4
	if h < 3 {
		return content
	}
	return lipgloss.NewStyle().Height(h).MaxHeight(h).Width(m.width).Render(content)
}

func (m *Health) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		status, sErr := m.client.Health(ctx)
		snap, snErr := m.client.Snapshot(ctx)

		if sErr != nil && snErr != nil {
			return errMsg{fmt.Errorf("health: %v, snapshot: %v", sErr, snErr)}
		}
		if sErr != nil {
			status = map[string]string{"status": "unknown"}
		}
		if snErr != nil {
			snap = inspect.Snapshot{}
		}
		return healthMsg{Status: status, Snapshot: snap}
	}
}

type healthMsg struct {
	Status   map[string]string
	Snapshot inspect.Snapshot
}
