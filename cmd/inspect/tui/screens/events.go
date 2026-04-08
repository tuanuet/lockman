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
	client        *client.Client
	events        []observe.Event
	loading       bool
	err           string
	filter        components.FilterModal
	appliedFilter client.Filter
	page          int
	pageSize      int
	width         int
	height        int
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
		m.filter = model
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter {
			m.appliedFilter = client.Filter{
				DefinitionID: m.filter.DefinitionID(),
				ResourceID:   m.filter.ResourceID(),
				OwnerID:      m.filter.OwnerID(),
				Kind:         client.ParseEventKind(m.filter.Kind()),
				Limit:        500,
			}
			m.page = 0
			m.filter.Hide()
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

	maxRows := m.height - 8
	if maxRows < 1 {
		maxRows = 1
	}
	pageLimit := start + maxRows
	if pageLimit < end {
		end = pageLimit
	}

	lines := []string{components.TitleStyle.Render("Events (F to filter, PgUp/PgDn to navigate)")}
	for _, e := range m.events[start:end] {
		color := components.RowStyle
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

func (m *Events) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		filter := m.appliedFilter
		if filter.Limit == 0 {
			filter.Limit = 500
		}
		events, err := m.client.Events(context.Background(), filter)
		if err != nil {
			return errMsg{err}
		}
		return eventsMsg{Events: events}
	}
}

func kindLabel(k observe.EventKind) string {
	return k.String()
}

type eventsMsg struct{ Events []observe.Event }
