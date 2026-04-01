package styles

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

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

	SubTab = lipgloss.NewStyle().
		Padding(0, 1).
		Foreground(DimGray)

	ActiveSubTab = lipgloss.NewStyle().
			Padding(0, 1).
			Bold(true).
			Foreground(Cyan)
)

// NewSpinner creates a consistently styled spinner.
func NewSpinner() spinner.Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(Red)
	return sp
}
