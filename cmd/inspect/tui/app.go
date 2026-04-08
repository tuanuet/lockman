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
			m.activeIdx = NextScreen(m.activeIdx, screenCount)
			return m, m.sendScreenCmd(ScreenRefreshMsg{})
		case tea.KeyShiftTab:
			m.activeIdx = PrevScreen(m.activeIdx, screenCount)
			return m, m.sendScreenCmd(ScreenRefreshMsg{})
		case tea.KeyEsc:
			if m.errToast != "" {
				m.errToast = ""
				return m, nil
			}
		}
		if msg.String() >= "1" && msg.String() <= "5" {
			m.activeIdx = int(msg.String()[0] - '1')
			return m, m.sendScreenCmd(ScreenRefreshMsg{})
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
