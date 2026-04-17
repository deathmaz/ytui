package app

import (
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
				if tab.browseID == "" {
					return nil
				}
				return &subscribeTarget{channelID: tab.browseID, name: tab.title}
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

// propagateSubscription fans a subscription state change to song tabs whose
// underlying video shares the channel. Artist tabs have no subscription
// indicator yet (about-style artist header is a separate step), so they are
// left alone — state is refetched on the next load.
func (m *MusicModel) propagateSubscription(channelID string, subscribed bool) {
	for i := range m.tabs.All() {
		tab := m.tabs.At(i)
		if tab.kind != musicTabSong {
			continue
		}
		if v := tab.songDetail.Video(); v != nil && v.ChannelID == channelID {
			tab.songDetail.SetChannelSubscribed(subscribed)
		}
	}
}
