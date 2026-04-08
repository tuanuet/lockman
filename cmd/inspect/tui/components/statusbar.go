package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var statusBarStyle = lipgloss.NewStyle().
	Foreground(White).
	Background(lipgloss.Color("#282a36")).
	Padding(0, 1)

func RenderStatusBar(hints []string, width int) string {
	content := ""
	for i, h := range hints {
		if i > 0 {
			content += "  "
		}
		content += h
	}
	return statusBarStyle.Width(width).Render(content)
}

func Hint(key, action string) string {
	return fmt.Sprintf("%s %s", key, action)
}
