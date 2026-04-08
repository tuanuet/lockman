package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/cmd/inspect/tui"
	"github.com/tuanuet/lockman/cmd/inspect/tui/components"
	"github.com/tuanuet/lockman/observe"
)

type Stream struct {
	client      *client.Client
	events      []observe.Event
	paused      bool
	loading     bool
	connected   bool
	err         string
	retries     int
	maxRetries  int
	ctx         context.Context
	cancel      context.CancelFunc
	program     *tea.Program
	filterInput textinput.Model
	streamDone  chan struct{}
	width       int
	height      int
}

func NewStream(c *client.Client) *Stream {
	ctx, cancel := context.WithCancel(context.Background())
	ti := textinput.New()
	ti.Placeholder = "Filter by definition_id..."
	ti.CharLimit = 64

	return &Stream{
		client:      c,
		maxRetries:  3,
		ctx:         ctx,
		cancel:      cancel,
		filterInput: ti,
	}
}

func (m *Stream) Init() tea.Cmd {
	m.stopStream()
	m.retries = 0
	m.err = ""
	m.ctx, m.cancel = context.WithCancel(context.Background())
	return m.startStream()
}

func (m *Stream) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tui.ScreenRefreshMsg:
		m.stopStream()
		m.retries = 0
		m.err = ""
		m.ctx, m.cancel = context.WithCancel(context.Background())
		return m, m.startStream()
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			m.stopStream()
			m.retries = 0
			m.err = ""
			m.ctx, m.cancel = context.WithCancel(context.Background())
			return m, m.startStream()
		case " ":
			m.paused = !m.paused
			return m, nil
		case "/":
			if m.filterInput.Focused() {
				m.filterInput.Blur()
			} else {
				m.filterInput.Focus()
			}
			return m, nil
		}
	}

	if m.filterInput.Focused() {
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case streamEventMsg:
		if !m.paused {
			m.events = append(m.events, msg.Event)
			if len(m.events) > 1000 {
				m.events = m.events[len(m.events)-1000:]
			}
		}
		return m, nil
	case streamErr:
		m.connected = false
		m.retries++
		if m.retries >= m.maxRetries {
			m.err = "Stream disconnected — press R to reconnect"
			return m, nil
		}
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
	if m.err != "" && len(m.events) == 0 && !m.connected {
		return components.ErrorStyle.Render(m.err)
	}

	header := components.TitleStyle.Render("Stream (Space: pause, R: reconnect, /: filter)")
	lines := []string{header}

	statusLine := components.DimStyle.Render("  ● Listening...")
	if m.paused {
		statusLine = components.WarnStyle.Render("  ■ Paused")
	} else if !m.connected {
		statusLine = components.ErrorStyle.Render("  ● Reconnecting...")
	}
	lines = append(lines, statusLine)

	if m.filterInput.Focused() {
		lines = append(lines, components.WarnStyle.Render("Filter: "+m.filterInput.View()))
	} else if m.filterInput.Value() != "" {
		lines = append(lines, components.DimStyle.Render("Filter: "+m.filterInput.Value()))
	}

	start := 0
	visible := m.height - 12
	if visible < 1 {
		visible = 1
	}
	if len(m.events) > visible {
		start = len(m.events) - visible
	}
	for _, e := range m.events[start:] {
		color := components.RowStyle
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

	if len(m.events) == 0 {
		lines = append(lines, "")
		lines = append(lines, components.DimStyle.Render("  Waiting for events from the server..."))
		lines = append(lines, components.DimStyle.Render("  Events will appear here as they occur."))
	}

	eventCount := fmt.Sprintf("  Total events received: %d", len(m.events))
	lines = append(lines, components.DimStyle.Render(eventCount))

	if m.err != "" && len(m.events) > 0 {
		lines = append(lines, components.ErrorStyle.Render(m.err))
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

func (m *Stream) stopStream() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.streamDone != nil {
		close(m.streamDone)
		m.streamDone = nil
	}
}

func (m *Stream) startStream() tea.Cmd {
	m.loading = true
	m.streamDone = make(chan struct{})
	return func() tea.Msg {
		eventCh, errCh := m.client.Stream(m.ctx)
		m.loading = false
		m.connected = true

		done := m.streamDone

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
				case <-done:
					return
				}
			}
		}()

		return nil
	}
}

func (m *Stream) SetProgram(p *tea.Program) {
	m.program = p
}

type streamEventMsg struct{ Event observe.Event }
type streamErr struct{ error }

func (e streamErr) Error() string { return e.error.Error() }
