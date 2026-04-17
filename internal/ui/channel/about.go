package channel

import (
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

const emptyAboutMsg = "(no channel data)"

// aboutView renders the About sub-tab: channel name, handle, subscriber and
// video counts, subscription indicator (hidden when unauthenticated), and
// description.
func aboutView(d *youtube.ChannelDetail, width int) string {
	if d == nil {
		return styles.Dim.Render(emptyAboutMsg)
	}
	var meta []string
	if d.SubscriberCount != "" {
		meta = append(meta, d.SubscriberCount)
	}
	if d.VideoCount != "" {
		meta = append(meta, d.VideoCount)
	}
	return shared.AboutView(shared.AboutData{
		Name:            d.Name,
		Subtitle:        d.Handle,
		MetaParts:       meta,
		Description:     d.Description,
		Subscribed:      d.Subscribed,
		SubscribedKnown: d.SubscribedKnown,
	}, width)
}
