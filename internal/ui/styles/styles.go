package styles

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	Red       = lipgloss.Color("#FF0000")
	DimGray   = lipgloss.Color("#666666")
	MidGray   = lipgloss.Color("#888888")
	LightGray = lipgloss.Color("#AAAAAA")
	DarkGray  = lipgloss.Color("#333333")
	White     = lipgloss.Color("#FFFFFF")
	Cyan      = lipgloss.Color("#00BFFF")
	Green     = lipgloss.Color("#00CC66")

	// Text styles
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(White)

	SelectedTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Cyan)

	Subtitle = lipgloss.NewStyle().
			Foreground(LightGray)

	Dim = lipgloss.NewStyle().
		Foreground(DimGray)

	Accent = lipgloss.NewStyle().
		Foreground(Red)

	Success = lipgloss.NewStyle().
		Foreground(Green)
)
