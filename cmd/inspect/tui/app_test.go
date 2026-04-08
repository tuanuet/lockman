package tui

import (
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/tuanuet/lockman/cmd/inspect/client"
)

type mockModel struct {
	initCalled bool
	updateMsgs []tea.Msg
}

func (m *mockModel) Init() tea.Cmd {
	m.initCalled = true
	return nil
}

func (m *mockModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.updateMsgs = append(m.updateMsgs, msg)
	return m, nil
}

func (m *mockModel) View() string {
	return "mock"
}

func TestApp_Init(t *testing.T) {
	screens := make([]tea.Model, screenCount)
	for i := range screens {
		screens[i] = &mockModel{}
	}
	app := NewApp(client.New("http://localhost"), screens)
	app.Init()

	for i, s := range screens {
		if !s.(*mockModel).initCalled {
			t.Errorf("screen %d Init() not called", i)
		}
	}
}

func TestApp_Navigation(t *testing.T) {
	screens := make([]tea.Model, screenCount)
	for i := range screens {
		screens[i] = &mockModel{}
	}
	app := NewApp(client.New("http://localhost"), screens)

	app.Update(tea.KeyMsg{Type: tea.KeyTab})
	if app.activeIdx != 1 {
		t.Errorf("after Tab, activeIdx = %d, want 1", app.activeIdx)
	}

	app.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if app.activeIdx != 0 {
		t.Errorf("after ShiftTab, activeIdx = %d, want 0", app.activeIdx)
	}

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if app.activeIdx != 2 {
		t.Errorf("after '3', activeIdx = %d, want 2", app.activeIdx)
	}
}

func TestApp_ScreenRefreshMsg(t *testing.T) {
	screens := make([]tea.Model, screenCount)
	for i := range screens {
		screens[i] = &mockModel{}
	}
	app := NewApp(client.New("http://localhost"), screens)
	app.Update(ScreenRefreshMsg{})

	m := screens[0].(*mockModel)
	if len(m.updateMsgs) == 0 {
		t.Fatal("expected screen to receive refresh message")
	}
	_, ok := m.updateMsgs[len(m.updateMsgs)-1].(ScreenRefreshMsg)
	if !ok {
		t.Errorf("expected ScreenRefreshMsg, got %T", m.updateMsgs[len(m.updateMsgs)-1])
	}
}
