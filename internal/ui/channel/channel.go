package channel

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/config"
	ytimage "github.com/deathmaz/ytui/internal/image"
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

// PlaylistsLoadedMsg carries channel playlist results.
type PlaylistsLoadedMsg struct {
	ChannelID string
	Playlists []youtube.Playlist
	NextToken string
	Append    bool
	Err       error
}

// PlaylistSelectedMsg is emitted when a user selects a playlist.
type PlaylistSelectedMsg struct {
	Playlist youtube.Playlist
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

	playlistList     list.Model
	plThumbList      *shared.ThumbList
	playlistToken    string
	playlistLoading  bool
	playlistLoaded   bool
	playlistLoadMore bool

	spinner spinner.Model
	client  youtube.Client
	width   int
	height  int
}

// New creates a new channel view model. Pass the same delegate and thumbList
// used by feed/search so the video list looks and works identically.
// The playlist list is set up internally with its own delegate and ThumbList
// sharing the same image renderer for cache reuse.
func New(client youtube.Client, videoDelegate list.ItemDelegate, thumbList *shared.ThumbList, cfg config.ThumbnailConfig) Model {
	plDelegate, plThumb := newPlaylistListSetup(thumbList, cfg)
	return Model{
		videoList:    shared.NewList(videoDelegate),
		thumbList:    thumbList,
		playlistList: shared.NewList(plDelegate),
		plThumbList:  plThumb,
		spinner:      styles.NewSpinner(),
		client:       client,
	}
}

func newPlaylistListSetup(videoThumbList *shared.ThumbList, cfg config.ThumbnailConfig) (list.ItemDelegate, *shared.ThumbList) {
	if !cfg.Enabled {
		return shared.PlaylistDelegate{}, nil
	}
	h := cfg.Height
	if h <= 0 {
		h = 5
	}
	// Reuse the image renderer from the video ThumbList for cache sharing.
	var imgR *ytimage.Renderer
	if videoThumbList != nil {
		imgR = videoThumbList.Renderer()
	}
	if imgR == nil {
		imgR = ytimage.NewRenderer()
	}
	plThumb := shared.NewThumbList(imgR, shared.PlaylistThumbURL)
	plDelegate := shared.NewThumbDelegate(imgR, h, shared.PlaylistThumbURL, shared.RenderPlaylistText)
	return plDelegate, plThumb
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
	m.playlistList.SetSize(w, listH)
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
	m.playlistLoaded = false
	m.playlistToken = ""

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

func (m *Model) loadPlaylists() tea.Cmd {
	if m.playlistLoading || m.playlistLoaded {
		return nil
	}
	m.playlistLoading = true
	client := m.client
	channelID := m.channel.ID
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		page, err := client.GetChannelPlaylists(context.Background(), channelID, "")
		if err != nil {
			return PlaylistsLoadedMsg{ChannelID: channelID, Err: err}
		}
		return PlaylistsLoadedMsg{
			ChannelID: channelID,
			Playlists: page.Items,
			NextToken: page.NextToken,
		}
	})
}

func (m *Model) loadMorePlaylists() tea.Cmd {
	if m.playlistLoadMore || m.playlistLoading || m.playlistToken == "" {
		return nil
	}
	m.playlistLoadMore = true
	token := m.playlistToken
	client := m.client
	channelID := m.channel.ID
	return func() tea.Msg {
		page, err := client.GetChannelPlaylists(context.Background(), channelID, token)
		if err != nil {
			return PlaylistsLoadedMsg{ChannelID: channelID, Err: err, Append: true}
		}
		return PlaylistsLoadedMsg{
			ChannelID: channelID,
			Playlists: page.Items,
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
		cmds = append(cmds, shared.AppendItems(&m.videoList, newItems, msg.Append))
		// Forward to video list so the delegate triggers thumbnail fetches.
		var cmd tea.Cmd
		m.videoList, cmd = m.videoList.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case PlaylistsLoadedMsg:
		if msg.ChannelID != m.channel.ID {
			return m, nil
		}
		m.playlistLoading = false
		m.playlistLoadMore = false
		m.playlistLoaded = true
		if msg.Err != nil {
			return m, nil
		}
		m.playlistToken = msg.NextToken

		var newItems []list.Item
		for _, p := range msg.Playlists {
			newItems = append(newItems, shared.PlaylistItem{Playlist: p})
		}
		cmds = append(cmds, shared.AppendItems(&m.playlistList, newItems, msg.Append))
		// Forward to playlist list so the delegate triggers thumbnail fetches.
		var cmd tea.Cmd
		m.playlistList, cmd = m.playlistList.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		switch {
		case msg.String() == "tab":
			m.activeTab = (m.activeTab + 1) % len(subTabNames)
			return m, m.onTabSwitch()
		case msg.String() == "shift+tab":
			m.activeTab = (m.activeTab - 1 + len(subTabNames)) % len(subTabNames)
			return m, m.onTabSwitch()
		}

		switch m.activeTab {
		case tabVideos:
			if !m.videoLoading {
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
		case tabPlaylists:
			if !m.playlistLoading {
				if key.Matches(msg, selectKey) {
					if item, ok := m.playlistList.SelectedItem().(shared.PlaylistItem); ok {
						return m, func() tea.Msg {
							return PlaylistSelectedMsg{Playlist: item.Playlist}
						}
					}
				}
				var cmd tea.Cmd
				m.playlistList, cmd = m.playlistList.Update(msg)
				cmds = append(cmds, cmd)
				if shared.ShouldLoadMore(m.playlistList, 5) {
					cmds = append(cmds, m.loadMorePlaylists())
				}
			}
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		if m.videoLoading || m.playlistLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	return m, tea.Batch(cmds...)
}

// onTabSwitch triggers lazy loading when switching to a sub-tab for the first time.
func (m *Model) onTabSwitch() tea.Cmd {
	switch m.activeTab {
	case tabPlaylists:
		return m.loadPlaylists()
	}
	return nil
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
		return m.renderPlaylists()
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

func (m Model) renderPlaylists() string {
	if m.playlistLoading && !m.playlistLoaded {
		return m.spinner.View() + " Loading playlists..."
	}
	return m.plThumbList.WrapView(shared.VisibleItems(m.playlistList), m.playlistList.View())
}
