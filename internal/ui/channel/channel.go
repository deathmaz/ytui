package channel

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

const subTabBarHeight = 1

const (
	tabVideos    = 0
	tabPlaylists = 1
	tabPosts     = 2
)

var (
	subTabNames = []string{"Videos", "Playlists", "Posts"}
	selectKey   = key.NewBinding(key.WithKeys("enter"))
)

// VideosLoadedMsg carries channel video results.
type VideosLoadedMsg struct {
	ChannelID string
	Videos    []youtube.Video
	NextToken string
	Append    bool
	Err       error
}

// Model is the channel detail view with Videos/Playlists/Posts sub-tabs.
type Model struct {
	activeTab int
	channel   youtube.Channel

	videoList     list.Model
	videoToken    string
	videoLoading  bool
	videoLoaded   bool
	videoLoadMore bool
	thumbList     *shared.ThumbList

	spinner spinner.Model
	client  youtube.Client
	width   int
	height  int
}

// New creates a new channel view model. Pass the same delegate and thumbList
// used by feed/search so the video list looks and works identically.
func New(client youtube.Client, delegate list.ItemDelegate, thumbList *shared.ThumbList) Model {
	return Model{
		videoList: shared.NewList(delegate),
		thumbList: thumbList,
		spinner:   styles.NewSpinner(),
		client:    client,
	}
}

func (m *Model) Channel() *youtube.Channel {
	return &m.channel
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	listH := h - subTabBarHeight
	if listH < 0 {
		listH = 0
	}
	m.videoList.SetSize(w, listH)
}

// SelectedVideo returns the currently selected video, if any.
func (m *Model) SelectedVideo() *youtube.Video {
	if m.activeTab != tabVideos {
		return nil
	}
	if item, ok := m.videoList.SelectedItem().(shared.VideoItem); ok {
		v := item.Video
		return &v
	}
	return nil
}

func (m *Model) Load(ch youtube.Channel) tea.Cmd {
	m.channel = ch
	m.activeTab = tabVideos
	m.videoLoading = true
	m.videoLoaded = false
	m.videoToken = ""

	client := m.client
	channelID := ch.ID
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		page, err := client.GetChannelVideos(context.Background(), channelID, "")
		if err != nil {
			return VideosLoadedMsg{ChannelID: channelID, Err: err}
		}
		return VideosLoadedMsg{
			ChannelID: channelID,
			Videos:    page.Items,
			NextToken: page.NextToken,
		}
	})
}

func (m *Model) loadMoreVideos() tea.Cmd {
	if m.videoLoadMore || m.videoLoading || m.videoToken == "" {
		return nil
	}
	m.videoLoadMore = true
	token := m.videoToken
	client := m.client
	channelID := m.channel.ID
	return func() tea.Msg {
		page, err := client.GetChannelVideos(context.Background(), channelID, token)
		if err != nil {
			return VideosLoadedMsg{ChannelID: channelID, Err: err, Append: true}
		}
		return VideosLoadedMsg{
			ChannelID: channelID,
			Videos:    page.Items,
			NextToken: page.NextToken,
			Append:    true,
		}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case VideosLoadedMsg:
		if msg.ChannelID != m.channel.ID {
			return m, nil
		}
		m.videoLoading = false
		m.videoLoadMore = false
		m.videoLoaded = true
		if msg.Err != nil {
			return m, nil
		}
		m.videoToken = msg.NextToken

		var newItems []list.Item
		for _, v := range msg.Videos {
			newItems = append(newItems, shared.VideoItem{Video: v})
		}
		if msg.Append {
			existing := m.videoList.Items()
			items := make([]list.Item, len(existing), len(existing)+len(newItems))
			copy(items, existing)
			items = append(items, newItems...)
			cmd := m.videoList.SetItems(items)
			cmds = append(cmds, cmd)
		} else {
			cmd := m.videoList.SetItems(newItems)
			cmds = append(cmds, cmd)
		}
		// Fall through to forward to list so the delegate triggers thumbnail fetches.

	case tea.KeyMsg:
		switch {
		case msg.String() == "tab":
			m.activeTab = (m.activeTab + 1) % len(subTabNames)
			return m, nil
		case msg.String() == "shift+tab":
			m.activeTab = (m.activeTab - 1 + len(subTabNames)) % len(subTabNames)
			return m, nil
		}

		if m.activeTab == tabVideos && !m.videoLoading {
			if key.Matches(msg, selectKey) {
				if item, ok := m.videoList.SelectedItem().(shared.VideoItem); ok {
					return m, func() tea.Msg {
						return shared.VideoSelectedMsg{Video: item.Video}
					}
				}
			}
			var cmd tea.Cmd
			m.videoList, cmd = m.videoList.Update(msg)
			cmds = append(cmds, cmd)

			if shared.ShouldLoadMore(m.videoList, 5) {
				cmds = append(cmds, m.loadMoreVideos())
			}
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		if m.videoLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	// Forward to list so the delegate's Update triggers thumbnail fetches.
	if m.activeTab == tabVideos {
		var cmd tea.Cmd
		m.videoList, cmd = m.videoList.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	header := shared.RenderSubTabBar(subTabNames, m.activeTab)
	content := m.renderActiveTab()
	return lipgloss.JoinVertical(lipgloss.Left, header, content)
}

func (m Model) renderActiveTab() string {
	switch m.activeTab {
	case tabVideos:
		return m.renderVideos()
	case tabPlaylists:
		return styles.Dim.Render("Playlists — coming soon")
	case tabPosts:
		return styles.Dim.Render("Posts — coming soon")
	}
	return ""
}

func (m Model) renderVideos() string {
	if m.videoLoading && !m.videoLoaded {
		return m.spinner.View() + fmt.Sprintf(" Loading videos for %s...", m.channel.Name)
	}
	return m.thumbList.WrapView(shared.VisibleItems(m.videoList), m.videoList.View())
}
