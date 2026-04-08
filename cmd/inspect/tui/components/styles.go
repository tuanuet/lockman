package components

import "github.com/charmbracelet/lipgloss"

var (
	Cyan   = lipgloss.Color("#8be9fd")
	Green  = lipgloss.Color("#50fa7b")
	Red    = lipgloss.Color("#ff5555")
	Yellow = lipgloss.Color("#f1fa8c")
	Gray   = lipgloss.Color("#6272a4")

	TitleStyle   = lipgloss.NewStyle().Foreground(Cyan).Bold(true)
	DimStyle     = lipgloss.NewStyle().Foreground(Gray)
	SuccessStyle = lipgloss.NewStyle().Foreground(Green)
	ErrorStyle   = lipgloss.NewStyle().Foreground(Red)
	WarnStyle    = lipgloss.NewStyle().Foreground(Yellow)
)
