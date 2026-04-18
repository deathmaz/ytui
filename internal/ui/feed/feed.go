package feed

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
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
	thumbList   *shared.ThumbList
}

// New creates a new feed view model.
func New(client youtube.Client, delegate list.ItemDelegate, thumbList *shared.ThumbList) Model {
	l := shared.NewList(delegate)

	return Model{
		list:      l,
		spinner:   styles.NewSpinner(),
		client:    client,
		thumbList: thumbList,
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
	// Invalidate thumbnail tracking so images are re-transmitted when the
	// list becomes visible again after the loading spinner.
	m.thumbList.Invalidate()
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

// RefetchThumbs returns a cmd that re-fetches thumbnails for visible items
// whose cache entries were evicted by the LRU.
func (m Model) RefetchThumbs() tea.Cmd {
	return m.thumbList.RefetchCmd(m.list)
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
		m.loaded = true
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.err = nil
		m.nextToken = msg.NextToken

		var newItems []list.Item
		for _, v := range msg.Videos {
			newItems = append(newItems, shared.VideoItem{Video: v})
		}
		cmds = append(cmds, shared.AppendItems(&m.list, newItems, msg.Append))
		// Fall through to trigger thumbnail fetches for new items.

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
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	// Forward to list so the delegate's Update triggers thumbnail fetches
	// (reached by FeedLoadedMsg fall-through and unknown message types).
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.loading {
		return m.thumbList.WrapView(nil, m.spinner.View()+" Loading subscription feed...")
	}
	if !m.loaded {
		return styles.Dim.Render("Press 'a' to authenticate to view feed")
	}
	if m.err != nil {
		return styles.Accent.Render("Feed error: "+m.err.Error()) +
			"\n\n" + styles.Dim.Render("Press 'a' to authenticate")
	}
	return m.thumbList.WrapView(shared.VisibleItems(m.list), m.list.View())
}
