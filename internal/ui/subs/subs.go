package subs

import (
	"context"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

var selectKey = key.NewBinding(key.WithKeys("enter"))

// SubsLoadedMsg carries subscription channel results.
type SubsLoadedMsg struct {
	Channels  []youtube.Channel
	NextToken string
	Append    bool
	Err       error
}

// ChannelSelectedMsg is emitted when a user selects a channel.
type ChannelSelectedMsg struct {
	Channel youtube.Channel
}

type channelItem struct {
	channel youtube.Channel
}

func (c channelItem) FilterValue() string { return c.channel.Name }
func (c channelItem) Title() string       { return c.channel.Name }
func (c channelItem) Description() string { return c.channel.Handle }

type channelDelegate struct{}

func (d channelDelegate) Height() int                             { return 2 }
func (d channelDelegate) Spacing() int                            { return 1 }
func (d channelDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d channelDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ci, ok := item.(channelItem)
	if !ok {
		return
	}

	ch := ci.channel
	isSelected := index == m.Index()

	cursor := "  "
	if isSelected {
		cursor = "> "
	}

	titleStyle := styles.Title
	if isSelected {
		titleStyle = styles.SelectedTitle
	}
	name := titleStyle.Render(shared.Truncate(ch.Name, m.Width()-4))

	var metaParts []string
	if ch.Handle != "" {
		metaParts = append(metaParts, ch.Handle)
	}
	if ch.SubscriberCount != "" {
		metaParts = append(metaParts, ch.SubscriberCount)
	}
	meta := styles.Dim.Render(strings.Join(metaParts, " · "))

	fmt.Fprintf(w, "%s%s\n%s  %s", cursor, name, "  ", meta)
}

// Model is the subscriptions list view.
type Model struct {
	list        list.Model
	spinner     spinner.Model
	loading     bool
	loaded      bool
	loadingMore bool
	nextToken   string
	err         error
	width       int
	height      int
	client      youtube.Client
}

// New creates a new subs view model.
func New(client youtube.Client) Model {
	l := shared.NewList(channelDelegate{})

	return Model{
		list:    l,
		spinner: styles.NewSpinner(),
		client:  client,
	}
}

// SetSize updates the view dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.list.SetSize(w, h)
}

// Channels returns a snapshot of the current channel list.
func (m *Model) Channels() []youtube.Channel {
	raw := m.list.Items()
	out := make([]youtube.Channel, 0, len(raw))
	for _, it := range raw {
		if ci, ok := it.(channelItem); ok {
			out = append(out, ci.channel)
		}
	}
	return out
}

// SelectedChannel returns the currently highlighted channel, if any.
func (m *Model) SelectedChannel() *youtube.Channel {
	ci, ok := m.list.SelectedItem().(channelItem)
	if !ok {
		return nil
	}
	c := ci.channel
	return &c
}

// RemoveChannel drops any item matching channelID from the list. Used to
// reflect an unsubscribe without refetching.
func (m *Model) RemoveChannel(channelID string) {
	existing := m.list.Items()
	filtered := existing[:0]
	for _, it := range existing {
		if ci, ok := it.(channelItem); ok && ci.channel.ID == channelID {
			continue
		}
		filtered = append(filtered, it)
	}
	if len(filtered) != len(existing) {
		m.list.SetItems(filtered)
	}
}

// Load fetches subscriptions. Only fetches once unless forced.
func (m *Model) Load(force bool) tea.Cmd {
	if m.loaded && !force {
		return nil
	}
	if m.loading {
		return nil
	}
	m.loading = true
	m.nextToken = ""
	client := m.client
	return tea.Batch(m.spinner.Tick, func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = SubsLoadedMsg{Err: fmt.Errorf("panic: %v", r)}
			}
		}()
		page, err := client.GetSubscriptions(context.Background(), "")
		if err != nil {
			return SubsLoadedMsg{Err: err}
		}
		return SubsLoadedMsg{Channels: page.Items, NextToken: page.NextToken}
	})
}

func (m *Model) loadMore() tea.Cmd {
	if m.loadingMore || m.loading || m.nextToken == "" {
		return nil
	}
	m.loadingMore = true
	token := m.nextToken
	client := m.client
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = SubsLoadedMsg{Err: fmt.Errorf("panic: %v", r)}
			}
		}()
		page, err := client.GetSubscriptions(context.Background(), token)
		if err != nil {
			return SubsLoadedMsg{Err: err}
		}
		return SubsLoadedMsg{Channels: page.Items, NextToken: page.NextToken, Append: true}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case SubsLoadedMsg:
		m.loading = false
		m.loadingMore = false
		m.loaded = true
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.err = nil
		m.nextToken = msg.NextToken

		var newItems []list.Item
		for _, ch := range msg.Channels {
			newItems = append(newItems, channelItem{channel: ch})
		}
		if msg.Append {
			existing := m.list.Items()
			items := make([]list.Item, len(existing), len(existing)+len(newItems))
			copy(items, existing)
			items = append(items, newItems...)
			cmd := m.list.SetItems(items)
			cmds = append(cmds, cmd)
		} else {
			cmd := m.list.SetItems(newItems)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		if !m.loading {
			if key.Matches(msg, selectKey) {
				if item, ok := m.list.SelectedItem().(channelItem); ok {
					return m, func() tea.Msg {
						return ChannelSelectedMsg{Channel: item.channel}
					}
				}
			}
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			cmds = append(cmds, cmd)

			if shared.ShouldLoadMore(m.list, 5) {
				cmds = append(cmds, m.loadMore())
			}
		}

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.loading {
		return m.spinner.View() + " Loading subscriptions..."
	}
	if !m.loaded {
		return styles.Dim.Render("Press 'a' to authenticate to view subscriptions")
	}
	if m.err != nil {
		return styles.Accent.Render("Subscriptions error: "+m.err.Error()) +
			"\n\n" + styles.Dim.Render("Press 'a' to authenticate")
	}
	return m.list.View()
}
