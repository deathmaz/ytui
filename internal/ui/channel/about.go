package channel

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

const (
	aboutDescPadding = 2
	aboutMinWidth    = 20
	emptyAboutMsg    = "(no channel data)"
)

// aboutView renders the About sub-tab: channel name, handle, subscriber and
// video counts, subscription indicator (hidden when unauthenticated), and
// description. width constrains long descriptions.
func aboutView(d *youtube.ChannelDetail, width int) string {
	if d == nil {
		return styles.Dim.Render(emptyAboutMsg)
	}

	var b strings.Builder
	title := styles.Title.Render(d.Name)
	if d.Handle != "" {
		title += "  " + styles.Subtitle.Render(d.Handle)
	}
	b.WriteString(title)
	b.WriteString("\n\n")

	var counts []string
	if d.SubscriberCount != "" {
		counts = append(counts, d.SubscriberCount)
	}
	if d.VideoCount != "" {
		counts = append(counts, d.VideoCount)
	}
	if len(counts) > 0 {
		b.WriteString(styles.Subtitle.Render(strings.Join(counts, " · ")))
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
