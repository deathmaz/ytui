package app

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/deathmaz/ytui/internal/ui/picker"
	"github.com/deathmaz/ytui/internal/youtube"
)

// resolveSubscribeTarget returns the channel the subscribe picker should act
// on in music mode. Priority: active song detail (has ChannelID), then active
// artist tab (browseID is the UC channel ID), then selected music item if
// it's an artist row. Returns nil if nothing applies.
func (m *MusicModel) resolveSubscribeTarget() *subscribeTarget {
	if !m.onFixedView {
		if tab := m.tabs.Active(); tab != nil {
			switch tab.kind {
			case musicTabSong:
				if v := tab.songDetail.Video(); v != nil && v.ChannelID != "" {
					return &subscribeTarget{
						channelID:  v.ChannelID,
						name:       v.ChannelName,
						known:      v.ChannelSubscribedKnown,
						subscribed: v.ChannelSubscribed,
					}
				}
			case musicTabArtist:
				// Subscribe requires a real UC channel ID. browseID on a
				// music artist is commonly prefixed (MPLA...) and will be
				// rejected by the endpoint, so we only use it as a last
				// resort when its shape matches a UC ID.
				t := &subscribeTarget{name: tab.title}
				if p := tab.artistPage; p != nil {
					t.channelID = p.ChannelID
					if p.Name != "" {
						t.name = p.Name
					}
					t.known = p.SubscribedKnown
					t.subscribed = p.Subscribed
				}
				if t.channelID == "" && strings.HasPrefix(tab.browseID, "UC") {
					t.channelID = tab.browseID
				}
				if t.channelID == "" {
					return nil
				}
				return t
			}
		}
	}

	if it := m.selectedMusicItem(); it != nil && it.Type == youtube.MusicArtist && it.BrowseID != "" {
		return &subscribeTarget{channelID: it.BrowseID, name: it.Title}
	}
	return nil
}

func (m *MusicModel) openSubscribePicker() tea.Cmd {
	if !m.ytClient.IsAuthenticated() {
		return m.setStatus("Authenticate first with '"+m.keys.Auth.Help().Key+"'", 3*time.Second)
	}
	target := m.resolveSubscribeTarget()
	if target == nil {
		return m.setStatus("No channel selected", 2*time.Second)
	}
	m.pendingSubscribe = target
	m.picker.Show(picker.TargetSubscribe, subscribePickerTitle, subscribeOptions(target), m.width, m.height)
	return nil
}

func (m *MusicModel) runSubscription(channelID, channelName string, subscribe bool) tea.Cmd {
	return runSubscriptionCmd(m.ytClient, m.setStatus, channelID, channelName, subscribe)
}

func (m *MusicModel) handleSubscribeResult(msg subscribeResultMsg) tea.Cmd {
	if msg.err == nil {
		m.propagateSubscription(msg.channelID, msg.subscribe)
	}
	text, dur := subscribeResultStatus(msg)
	return m.setStatus(text, dur)
}

// propagateSubscription fans a subscription state change out to every open
// music tab that references the channel: song tabs via the shared
// detail.Model, artist tabs via the About header. Artist tabs match on the
// parsed ChannelID (falling back to browseID when it equals the UC ID).
func (m *MusicModel) propagateSubscription(channelID string, subscribed bool) {
	for i := range m.tabs.All() {
		tab := m.tabs.At(i)
		switch tab.kind {
		case musicTabSong:
			if v := tab.songDetail.Video(); v != nil && v.ChannelID == channelID {
				tab.songDetail.SetChannelSubscribed(subscribed)
			}
		case musicTabArtist:
			p := tab.artistPage
			if p == nil || p.ChannelID == "" {
				continue
			}
			if p.ChannelID == channelID {
				p.Subscribed = subscribed
				p.SubscribedKnown = true
			}
		}
	}
}
