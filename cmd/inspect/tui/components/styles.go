package components

import "github.com/charmbracelet/lipgloss"

var (
	Cyan   = lipgloss.Color("#8be9fd")
	Green  = lipgloss.Color("#50fa7b")
	Red    = lipgloss.Color("#ff5555")
	Yellow = lipgloss.Color("#f1fa8c")
	Gray   = lipgloss.Color("#8b8fa3")
	White  = lipgloss.Color("#f8f8f2")

	TitleStyle     = lipgloss.NewStyle().Foreground(Cyan).Bold(true)
	DimStyle       = lipgloss.NewStyle().Foreground(Gray)
	SuccessStyle   = lipgloss.NewStyle().Foreground(Green)
	ErrorStyle     = lipgloss.NewStyle().Foreground(Red)
	WarnStyle      = lipgloss.NewStyle().Foreground(Yellow)
	RowStyle       = lipgloss.NewStyle().Foreground(White)
	SelectedStyle  = lipgloss.NewStyle().Foreground(Cyan).Bold(true)
	BorderStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Gray).Padding(0, 1)
	ColumnStyle    = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#44475a")).Padding(0, 1)
	BottomBarStyle = lipgloss.NewStyle().Foreground(White).Padding(0, 1)
)
