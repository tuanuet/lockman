package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	tabActiveStyle   = lipgloss.NewStyle().Foreground(Cyan).Bold(true)
	tabInactiveStyle = lipgloss.NewStyle().Foreground(White)
)

func RenderTabBar(screenNames []string, activeIdx int, width int) string {
	tabs := make([]string, len(screenNames))
	for i, name := range screenNames {
		label := name
		if i == activeIdx {
			label = "● " + name
			tabs[i] = tabActiveStyle.Render(label)
		} else {
			tabs[i] = tabInactiveStyle.Render(label)
		}
	}
	bar := strings.Join(tabs, "  ")
	return lipgloss.NewStyle().
		Width(width).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Render(bar)
}
