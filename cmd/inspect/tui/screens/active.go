package screens

import (
	"context"
	"fmt"
	"sort"
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
		case "enter":
			if m.selected >= 0 && m.selected < len(m.locks) {
				l := m.locks[m.selected]
				detail := fmt.Sprintf("Lock: %s/%s Owner: %s Acquired: %s",
					l.DefinitionID, l.ResourceID, l.OwnerID, l.AcquiredAt.Format("15:04:05"))
				return m, func() tea.Msg { return tui.ErrToastMsg(detail) }
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
		{Title: "Acquired At", Width: 12},
		{Title: "Duration", Width: 12},
	}

	var rows [][]string
	for _, l := range m.locks {
		dur := time.Since(l.AcquiredAt).Round(time.Second)
		rows = append(rows, []string{
			l.DefinitionID,
			l.ResourceID,
			l.OwnerID,
			l.AcquiredAt.Format("15:04:05"),
			dur.String(),
		})
	}

	header := components.TitleStyle.Render("Active Locks (S to sort, ↑/↓ to navigate, Enter: details)")
	table := components.Table(columns, rows, m.selected)

	content := header + "\n" + table
	if m.height == 0 {
		return content
	}

	h := m.height - 4
	if h < 3 {
		return content
	}
	return lipgloss.NewStyle().Height(h).MaxHeight(h).Width(m.width).Render(content)
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
