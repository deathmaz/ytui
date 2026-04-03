package subs

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

var selectKey = key.NewBinding(key.WithKeys("enter"))

// SubsLoadedMsg carries subscription channel results.
type SubsLoadedMsg struct {
	Channels []youtube.Channel
	Err      error
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
	list    list.Model
	spinner spinner.Model
	loading bool
	loaded  bool
	err     error
	width   int
	height  int
	client  youtube.Client
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

// Load fetches subscriptions. Only fetches once unless forced.
func (m *Model) Load(force bool) tea.Cmd {
	if m.loaded && !force {
		return nil
	}
	if m.loading {
		return nil
	}
	m.loading = true
	client := m.client
	return tea.Batch(m.spinner.Tick, func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = SubsLoadedMsg{Err: fmt.Errorf("panic: %v", r)}
			}
		}()
		// Fetch all pages
		var allChannels []youtube.Channel
		pageToken := ""
		for {
			page, err := client.GetSubscriptions(context.Background(), pageToken)
			if err != nil {
				return SubsLoadedMsg{Err: err}
			}
			allChannels = append(allChannels, page.Items...)
			if !page.HasMore {
				break
			}
			pageToken = page.NextToken
		}
		return SubsLoadedMsg{Channels: allChannels}
	})
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case SubsLoadedMsg:
		m.loading = false
		m.loaded = true
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.err = nil
		items := make([]list.Item, len(msg.Channels))
		for i, ch := range msg.Channels {
			items[i] = channelItem{channel: ch}
		}
		cmd := m.list.SetItems(items)
		cmds = append(cmds, cmd)
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
