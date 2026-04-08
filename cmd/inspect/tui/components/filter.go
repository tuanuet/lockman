package components

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type FilterModal struct {
	visible   bool
	defInput  textinput.Model
	resInput  textinput.Model
	ownInput  textinput.Model
	kindInput textinput.Model
	focused   int
}

func NewFilterModal() FilterModal {
	ti := func(placeholder string) textinput.Model {
		m := textinput.New()
		m.Placeholder = placeholder
		m.CharLimit = 64
		return m
	}
	m := FilterModal{
		defInput:  ti("definition_id"),
		resInput:  ti("resource_id"),
		ownInput:  ti("owner_id"),
		kindInput: ti("kind (e.g. contention)"),
		focused:   0,
	}
	m.defInput.Focus()
	return m
}

func (m *FilterModal) Show() {
	m.visible = true
	m.defInput.Focus()
	m.focused = 0
}

func (m *FilterModal) Hide() {
	m.visible = false
}

func (m *FilterModal) Visible() bool {
	return m.visible
}

func (m *FilterModal) Update(msg tea.Msg) (FilterModal, tea.Cmd) {
	if !m.visible {
		return *m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			m.Hide()
			return *m, nil
		case tea.KeyTab, tea.KeyShiftTab:
			m.focused = (m.focused + 1) % 4
			inputs := []*textinput.Model{&m.defInput, &m.resInput, &m.ownInput, &m.kindInput}
			for i, inp := range inputs {
				if i == m.focused {
					inp.Focus()
				} else {
					inp.Blur()
				}
			}
		case tea.KeyEnter:
			m.Hide()
			return *m, nil
		}
	}
	var cmd tea.Cmd
	inputs := []*textinput.Model{&m.defInput, &m.resInput, &m.ownInput, &m.kindInput}
	for i, inp := range inputs {
		if i == m.focused {
			*inp, cmd = inp.Update(msg)
		}
	}
	return *m, cmd
}

func (m *FilterModal) View() string {
	if !m.visible {
		return ""
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		BorderForeground(Cyan)

	content := lipgloss.JoinVertical(lipgloss.Left,
		"Filter Events",
		"Definition: "+m.defInput.View(),
		"Resource:   "+m.resInput.View(),
		"Owner:      "+m.ownInput.View(),
		"Kind:       "+m.kindInput.View(),
		"Tab: next field | Enter: apply | Esc: cancel",
	)
	return style.Render(content)
}

func (m *FilterModal) DefinitionID() string { return m.defInput.Value() }
func (m *FilterModal) ResourceID() string   { return m.resInput.Value() }
func (m *FilterModal) OwnerID() string      { return m.ownInput.Value() }
func (m *FilterModal) Kind() string         { return m.kindInput.Value() }
