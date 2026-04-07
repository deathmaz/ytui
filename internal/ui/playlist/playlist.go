package playlist

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

// VideosLoadedMsg carries playlist video results.
type VideosLoadedMsg struct {
	PlaylistID string
	Videos     []youtube.Video
	NextToken  string
	Append     bool
	Err        error
}

// Model is the playlist video list view.
type Model struct {
	playlist   youtube.Playlist
	list       list.Model
	thumbList  *shared.ThumbList
	spinner    spinner.Model
	loading    bool
	loaded     bool
	loadMore   bool
	nextToken  string
	client     youtube.Client
	width      int
	height     int
}

// New creates a new playlist view. Pass the same delegate and thumbList
// used by feed/search/channel for consistent video list rendering.
func New(client youtube.Client, delegate list.ItemDelegate, thumbList *shared.ThumbList) Model {
	return Model{
		list:      shared.NewList(delegate),
		thumbList: thumbList,
		spinner:   styles.NewSpinner(),
		client:    client,
	}
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.list.SetSize(w, h)
}

// SelectedVideo returns the currently selected video, if any.
func (m *Model) SelectedVideo() *youtube.Video {
	if item, ok := m.list.SelectedItem().(shared.VideoItem); ok {
		v := item.Video
		return &v
	}
	return nil
}

// Refresh reloads the playlist videos.
func (m *Model) Refresh() tea.Cmd {
	return m.Load(m.playlist)
}

func (m *Model) Load(pl youtube.Playlist) tea.Cmd {
	m.playlist = pl
	m.loading = true
	m.loaded = false
	m.nextToken = ""

	client := m.client
	playlistID := pl.ID
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		page, err := client.GetPlaylistVideos(context.Background(), playlistID, "")
		if err != nil {
			return VideosLoadedMsg{PlaylistID: playlistID, Err: err}
		}
		return VideosLoadedMsg{
			PlaylistID: playlistID,
			Videos:     page.Items,
			NextToken:  page.NextToken,
		}
	})
}

func (m *Model) loadMoreVideos() tea.Cmd {
	if m.loadMore || m.loading || m.nextToken == "" {
		return nil
	}
	m.loadMore = true
	token := m.nextToken
	client := m.client
	playlistID := m.playlist.ID
	return func() tea.Msg {
		page, err := client.GetPlaylistVideos(context.Background(), playlistID, token)
		if err != nil {
			return VideosLoadedMsg{PlaylistID: playlistID, Err: err, Append: true}
		}
		return VideosLoadedMsg{
			PlaylistID: playlistID,
			Videos:     page.Items,
			NextToken:  page.NextToken,
			Append:     true,
		}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case VideosLoadedMsg:
		if msg.PlaylistID != m.playlist.ID {
			return m, nil
		}
		m.loading = false
		m.loadMore = false
		m.loaded = true
		if msg.Err != nil {
			return m, nil
		}
		m.nextToken = msg.NextToken

		var newItems []list.Item
		for _, v := range msg.Videos {
			newItems = append(newItems, shared.VideoItem{Video: v})
		}
		cmds = append(cmds, shared.AppendItems(&m.list, newItems, msg.Append))
		// Fall through to forward to list so the delegate triggers thumbnail fetches.

	case tea.KeyMsg:
		if !m.loading {
			if key.Matches(msg, selectKey) {
				if item, ok := m.list.SelectedItem().(shared.VideoItem); ok {
					return m, func() tea.Msg {
						return shared.VideoSelectedMsg{Video: item.Video}
					}
				}
			}
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			cmds = append(cmds, cmd)

			if shared.ShouldLoadMore(m.list, 5) {
				cmds = append(cmds, m.loadMoreVideos())
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

	// Forward to list so the delegate's Update triggers thumbnail fetches.
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.loading && !m.loaded {
		return m.spinner.View() + fmt.Sprintf(" Loading playlist %s...", m.playlist.Title)
	}
	return m.thumbList.WrapView(shared.VisibleItems(m.list), m.list.View())
}
