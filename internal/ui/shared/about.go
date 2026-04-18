package shared

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/deathmaz/ytui/internal/ui/styles"
)

// AboutData is the input to AboutView. Callers populate whichever fields
// apply to their domain; empty fields are skipped during rendering.
type AboutData struct {
	Name        string
	Subtitle    string   // rendered after Name with a 2-space separator
	MetaParts   []string // joined with " · " onto a single line
	Description string

	// Subscribed is the signed-in user's state. SubscribedKnown distinguishes
	// "not subscribed" from an unauthenticated response where no state is
	// available — in the latter case no indicator is rendered.
	Subscribed      bool
	SubscribedKnown bool
}

const (
	aboutDescPadding = 2
	aboutMinWidth    = 20
)

// AboutView renders a header block shared by channel and music-artist
// About sub-tabs: name + optional subtitle, one meta line, subscription
// indicator, width-wrapped description.
func AboutView(d AboutData, width int) string {
	var b strings.Builder
	b.WriteString(styles.Title.Render(d.Name))
	if d.Subtitle != "" {
		b.WriteString("  ")
		b.WriteString(styles.Subtitle.Render(d.Subtitle))
	}
	b.WriteString("\n\n")

	if len(d.MetaParts) > 0 {
		b.WriteString(styles.Subtitle.Render(strings.Join(d.MetaParts, " · ")))
		b.WriteString("\n")
	}

	if ind := styles.SubscriptionIndicator(d.SubscribedKnown, d.Subscribed); ind != "" {
		b.WriteString(ind)
		b.WriteString("\n")
	}

	if d.Description != "" {
		b.WriteString("\n")
		w := width - aboutDescPadding
		if w < aboutMinWidth {
			w = aboutMinWidth
		}
		b.WriteString(lipgloss.NewStyle().Width(w).Render(d.Description))
		b.WriteString("\n")
	}

	return b.String()
}
