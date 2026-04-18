package styles

import (
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
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

	Tab = lipgloss.NewStyle().
		Padding(0, 2)

	ActiveTab = lipgloss.NewStyle().
			Padding(0, 2).
			Bold(true).
			Foreground(Red)

	StatusBar = lipgloss.NewStyle().
			Foreground(DimGray)

	TabSeparator = lipgloss.NewStyle().
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(DarkGray)
)

// SubscriptionIndicator returns a styled "subscribed" / "not subscribed" label
// for use in channel-About, video-detail, and artist views. Returns the empty
// string when the signed-in subscription state is unknown (e.g. unauthenticated).
func SubscriptionIndicator(known, subscribed bool) string {
	if !known {
		return ""
	}
	if subscribed {
		return Success.Render("✓ Subscribed")
	}
	return Dim.Render("○ Not subscribed")
}

// NewSpinner creates a consistently styled spinner.
func NewSpinner() spinner.Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(Red)
	return sp
}
