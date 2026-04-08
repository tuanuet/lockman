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
