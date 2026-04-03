package feed

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

var selectKey = key.NewBinding(key.WithKeys("enter"))

// FeedLoadedMsg carries feed results.
type FeedLoadedMsg struct {
	Videos    []youtube.Video
	NextToken string
	Append    bool
	Err       error
}

// VideoSelectedMsg is emitted when a user selects a video from the feed.
type VideoSelectedMsg = shared.VideoSelectedMsg

// Model is the subscription feed view.
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

// New creates a new feed view model.
func New(client youtube.Client) Model {
	l := shared.NewList(shared.VideoDelegate{})

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

// Load fetches the subscription feed. Only fetches once unless forced.
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
				msg = FeedLoadedMsg{Err: fmt.Errorf("panic: %v", r)}
			}
		}()
		page, err := client.GetFeed(context.Background(), "")
		if err != nil {
			return FeedLoadedMsg{Err: err}
		}
		return FeedLoadedMsg{Videos: page.Items, NextToken: page.NextToken}
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
				msg = FeedLoadedMsg{Err: fmt.Errorf("panic: %v", r)}
			}
		}()
		page, err := client.GetFeed(context.Background(), token)
		if err != nil {
			return FeedLoadedMsg{Err: err}
		}
		return FeedLoadedMsg{Videos: page.Items, NextToken: page.NextToken, Append: true}
	}
}

// SelectedVideo returns the currently selected video.
func (m Model) SelectedVideo() (youtube.Video, bool) {
	if item, ok := m.list.SelectedItem().(shared.VideoItem); ok {
		return item.Video, true
	}
	return youtube.Video{}, false
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case FeedLoadedMsg:
		m.loading = false
		m.loadingMore = false
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.err = nil
		m.loaded = true
		m.nextToken = msg.NextToken

		var newItems []list.Item
		for _, v := range msg.Videos {
			newItems = append(newItems, shared.VideoItem{Video: v})
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
				if item, ok := m.list.SelectedItem().(shared.VideoItem); ok {
					return m, func() tea.Msg {
						return VideoSelectedMsg{Video: item.Video}
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
		return m.spinner.View() + " Loading subscription feed..."
	}
	if !m.loaded {
		return styles.Dim.Render("Press 'a' to authenticate to view feed")
	}
	if m.err != nil {
		return styles.Accent.Render("Feed error: "+m.err.Error()) +
			"\n\n" + styles.Dim.Render("Press 'a' to authenticate")
	}
	return m.list.View()
}
