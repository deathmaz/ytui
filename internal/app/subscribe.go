package app

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/deathmaz/ytui/internal/ui/picker"
)

// subscribeResultMsg is dispatched after a Subscribe/Unsubscribe call completes.
type subscribeResultMsg struct {
	channelID   string
	channelName string
	subscribe   bool
	err         error
}

const subscribePickerTitle = "Subscription"

const (
	subKeySubscribe   = "subscribe"
	subKeyUnsubscribe = "unsubscribe"
)

// subscribeTarget captures enough to identify the channel we will act on and
// to render a useful status line afterwards.
type subscribeTarget struct {
	channelID string
	name      string

	// known is true when the current subscription state is known; subscribed
	// is only meaningful when known. Used to preselect/annotate the picker.
	known      bool
	subscribed bool
}

// resolveSubscribeTarget inspects the active view/tab and returns the channel
// the subscribe picker should act on, or nil if none is applicable.
func (m *Model) resolveSubscribeTarget() *subscribeTarget {
	if m.activeView == ViewDynamicTab {
		if tab := m.tabs.Active(); tab != nil {
			switch tab.kind {
			case tabVideo:
				if v := tab.detail.Video(); v != nil && v.ChannelID != "" {
					return &subscribeTarget{
						channelID:  v.ChannelID,
						name:       v.ChannelName,
						known:      v.ChannelSubscribedKnown,
						subscribed: v.ChannelSubscribed,
					}
				}
			case tabChannel:
				ch := tab.channel.Channel()
				if ch == nil || ch.ID == "" {
					return nil
				}
				target := &subscribeTarget{channelID: ch.ID, name: ch.Name}
				if d := tab.channel.Detail(); d != nil {
					target.known = d.SubscribedKnown
					target.subscribed = d.Subscribed
				}
				return target
			}
		}
	}

	// Fall back to the selected video's channel for feed/search/detail views.
	if v := m.selectedVideo(); v != nil && v.ChannelID != "" {
		return &subscribeTarget{
			channelID:  v.ChannelID,
			name:       v.ChannelName,
			known:      v.ChannelSubscribedKnown,
			subscribed: v.ChannelSubscribed,
		}
	}

	if m.activeView == ViewSubs {
		if ch := m.subs.SelectedChannel(); ch != nil {
			// Subs list only contains channels the user is already subscribed to.
			return &subscribeTarget{channelID: ch.ID, name: ch.Name, known: true, subscribed: true}
		}
	}

	return nil
}

func (m *Model) openSubscribePicker() tea.Cmd {
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

// subscribeOptions returns the picker rows. When the current state is known,
// only the transitioning action is shown — no use letting the user pick a
// no-op. When state is unknown, both options appear as a safe fallback.
func subscribeOptions(t *subscribeTarget) []picker.Option {
	sub := picker.Option{Key: subKeySubscribe, Label: "Subscribe"}
	unsub := picker.Option{Key: subKeyUnsubscribe, Label: "Unsubscribe"}
	if !t.known {
		return []picker.Option{sub, unsub}
	}
	if t.subscribed {
		return []picker.Option{unsub}
	}
	return []picker.Option{sub}
}

// runSubscription drives the actual Subscribe/Unsubscribe call.
func (m *Model) runSubscription(channelID, channelName string, subscribe bool) tea.Cmd {
	if channelName == "" {
		channelName = channelID
	}
	client := m.ytClient
	pending := "Subscribing to " + channelName + "..."
	if !subscribe {
		pending = "Unsubscribing from " + channelName + "..."
	}
	return tea.Batch(
		m.setStatus(pending, 10*time.Second),
		func() tea.Msg {
			ctx := context.Background()
			var err error
			if subscribe {
				err = client.Subscribe(ctx, channelID)
			} else {
				err = client.Unsubscribe(ctx, channelID)
			}
			return subscribeResultMsg{
				channelID:   channelID,
				channelName: channelName,
				subscribe:   subscribe,
				err:         err,
			}
		},
	)
}

// handleSubscribeResult applies a successful subscribe/unsubscribe to every
// open tab that references the channel and to the subs list, then shows a
// status message. Errors surface as a status message only.
func (m *Model) handleSubscribeResult(msg subscribeResultMsg) tea.Cmd {
	if msg.err != nil {
		verb := "Subscribe"
		if !msg.subscribe {
			verb = "Unsubscribe"
		}
		return m.setStatus(fmt.Sprintf("%s failed: %v", verb, msg.err), 5*time.Second)
	}
	m.propagateSubscription(msg.channelID, msg.subscribe)
	verb := "Subscribed to "
	if !msg.subscribe {
		verb = "Unsubscribed from "
	}
	return m.setStatus(verb+msg.channelName, 3*time.Second)
}

// propagateSubscription fans a subscription state change out to every open
// tab or view that references channelID.
func (m *Model) propagateSubscription(channelID string, subscribed bool) {
	for i := range m.tabs.All() {
		tab := m.tabs.At(i)
		switch tab.kind {
		case tabChannel:
			if ch := tab.channel.Channel(); ch != nil && ch.ID == channelID {
				tab.channel.SetSubscribed(subscribed)
			}
		case tabVideo:
			if v := tab.detail.Video(); v != nil && v.ChannelID == channelID {
				tab.detail.SetChannelSubscribed(subscribed)
			}
		}
	}
	// Unsubscribe removes the row from the already-loaded subs list so the
	// user sees the effect without a manual refresh. Subscribe has no symmetric
	// insertion path (we don't know the channel's full row data) — the user
	// will see it appear on next refresh.
	if !subscribed {
		m.subs.RemoveChannel(channelID)
	}
}

