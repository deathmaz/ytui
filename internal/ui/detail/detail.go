package detail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/ui/comments"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

var (
	separatorStyle = styles.Dim
	contentStyle   = styles.Subtitle

	goTopKey    = key.NewBinding(key.WithKeys("g", "home"))
	goBottomKey = key.NewBinding(key.WithKeys("G", "end"))
)

const (
	thumbCols = 40
	thumbRows = 10

	tabInfo     = 0
	tabComments = 1
)

// VideoLoadedMsg carries the loaded video details.
type VideoLoadedMsg struct {
	Video *youtube.Video
	Err   error
}

type clearTransmitMsg struct{}

// Model is the video detail view with Info and Comments sub-tabs.
type Model struct {
	activeTab    int // 0=Info, 1=Comments
	infoViewport viewport.Model
	comments     comments.Model
	spinner      spinner.Model
	video        *youtube.Video
	loading      bool
	width        int
	height       int
	client       youtube.Client
	imgR         *ytimage.Renderer
	thumbTransmit string
	thumbPlace    string
	thumbPending  bool
	thumbFailed   bool
}

// New creates a new detail view model.
func New(client youtube.Client, imgR *ytimage.Renderer) Model {
	cm := comments.New("video", func(ctx context.Context, token string) (*youtube.Page[youtube.Comment], error) {
		return client.GetComments(ctx, "", token)
	}, func(ctx context.Context, commentID, token string) (*youtube.Page[youtube.Comment], error) {
		return client.GetReplies(ctx, commentID, token)
	})
	return Model{
		spinner:  styles.NewSpinner(),
		comments: cm,
		client:   client,
		imgR:     imgR,
	}
}

const subTabBarHeight = 1

func (m *Model) viewportHeight() int {
	h := m.height - subTabBarHeight
	if h < 1 {
		h = 1
	}
	return h
}

// SetSize updates the view dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	vh := m.viewportHeight()
	m.infoViewport.Width = w
	m.infoViewport.Height = vh
	m.comments.SetSize(w, vh)
	if m.video != nil && m.activeTab == tabInfo {
		m.infoViewport.SetContent(m.renderInfo())
	}
}

// LoadVideo starts loading a video's details.
func (m *Model) LoadVideo(id string) tea.Cmd {
	m.loading = true
	m.activeTab = tabInfo
	m.video = nil
	m.thumbTransmit = ""
	m.thumbPlace = ""
	m.thumbPending = false
	m.thumbFailed = false
	client := m.client
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		v, err := client.GetVideo(context.Background(), id)
		return VideoLoadedMsg{Video: v, Err: err}
	})
}

// Video returns the currently loaded video.
func (m Model) Video() *youtube.Video {
	return m.video
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case VideoLoadedMsg:
		m.loading = false
		vh := m.viewportHeight()
		if msg.Err != nil {
			m.infoViewport = viewport.New(m.width, vh)
			m.infoViewport.KeyMap = viewportKeyMap()
			m.infoViewport.SetContent(fmt.Sprintf("Error loading video: %v", msg.Err))
			return m, nil
		}
		m.video = msg.Video
		m.infoViewport = viewport.New(m.width, vh)
		m.infoViewport.KeyMap = viewportKeyMap()

		// Start thumbnail fetch
		if m.imgR != nil {
			thumbURL := shared.BestThumbnail(*m.video)
			if thumbURL != "" {
				tx, pl := m.imgR.Get(thumbURL)
				if pl != "" {
					m.thumbTransmit = tx
					m.thumbPlace = pl
					cmds = append(cmds, scheduleClearTransmit())
				} else {
					m.thumbPending = true
					cmds = append(cmds, m.imgR.FetchCmd(thumbURL, thumbCols, thumbRows))
				}
			}
		}

		// Auto-fetch comments
		if m.video.CommentsToken != "" {
			cmds = append(cmds, m.comments.Load(m.video.CommentsToken))
		}

		m.infoViewport.SetContent(m.renderInfo())
		return m, tea.Batch(cmds...)

	case ytimage.ThumbnailLoadedMsg:
		// Only process if this fetch was initiated by our renderer
		// (not by a list thumbnail renderer sharing the message bus).
		if m.imgR != nil && m.imgR.HandleLoaded(msg) {
			m.thumbPending = false
			if msg.Err == nil && msg.Placeholder != "" {
				m.thumbTransmit = msg.TransmitStr
				m.thumbPlace = msg.Placeholder
				cmds = append(cmds, scheduleClearTransmit())
			} else {
				m.thumbFailed = true
			}
			if m.video != nil {
				m.infoViewport.SetContent(m.renderInfo())
			}
		}
		return m, tea.Batch(cmds...)

	case clearTransmitMsg:
		m.thumbTransmit = ""
		return m, nil

	case comments.LoadedMsg, comments.RepliesLoadedMsg:
		var cmd tea.Cmd
		m.comments, cmd = m.comments.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if !m.loading {
			// Tab switching
			if msg.String() == "tab" {
				if m.activeTab == tabInfo {
					m.activeTab = tabComments
				}
				return m, nil
			}
			if msg.String() == "shift+tab" {
				if m.activeTab == tabComments {
					m.activeTab = tabInfo
				}
				return m, nil
			}

			if m.activeTab == tabInfo {
				switch {
				case key.Matches(msg, goTopKey):
					m.infoViewport.GotoTop()
					return m, nil
				case key.Matches(msg, goBottomKey):
					m.infoViewport.GotoBottom()
					return m, nil
				default:
					var cmd tea.Cmd
					m.infoViewport, cmd = m.infoViewport.Update(msg)
					cmds = append(cmds, cmd)
				}
			} else {
				var cmd tea.Cmd
				m.comments, cmd = m.comments.Update(msg)
				return m, cmd
			}
		}

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		// Forward to comments for its own spinner
		var cmd tea.Cmd
		m.comments, cmd = m.comments.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func scheduleClearTransmit() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return clearTransmitMsg{}
	})
}

var subTabNames = []string{"Info", "Comments"}

func (m Model) renderSubTabBar() string {
	return shared.RenderSubTabBar(subTabNames, m.activeTab)
}

func (m Model) View() string {
	if m.loading {
		return m.spinner.View() + " Loading video details..."
	}

	subBar := m.renderSubTabBar()
	var content string
	if m.activeTab == tabInfo {
		content = m.infoViewport.View()
	} else {
		content = m.comments.View()
	}

	view := lipgloss.JoinVertical(lipgloss.Left, subBar, content)
	if m.thumbTransmit != "" {
		view = m.thumbTransmit + view
	}
	return view
}


// renderInfo renders the Info tab content.
func (m *Model) renderInfo() string {
	v := m.video
	if v == nil {
		return ""
	}

	var b strings.Builder
	sep := separatorStyle.Render(strings.Repeat("─", m.width-2))

	// Thumbnail
	if m.thumbPlace != "" {
		b.WriteString(m.thumbPlace)
		b.WriteString("\n\n")
	} else if m.thumbPending {
		for i := 0; i < thumbRows+1; i++ {
			b.WriteString("\n")
		}
	}

	// Title
	b.WriteString(styles.Title.MarginBottom(1).Width(m.width - 2).Render(v.Title))
	b.WriteString("\n")

	// Channel
	channel := styles.Accent.Bold(true).Render(v.ChannelName)
	if v.SubscriberCount != "" {
		channel += styles.Subtitle.Render("  " + v.SubscriberCount)
	}
	b.WriteString(channel)
	b.WriteString("\n\n")

	// Stats
	var stats []string
	if v.ViewCount != "" {
		stats = append(stats, styles.Accent.Bold(true).Render("Views: ")+styles.Subtitle.Render(v.ViewCount))
	}
	if v.LikeCount != "" {
		stats = append(stats, styles.Accent.Bold(true).Render("Likes: ")+styles.Subtitle.Render(v.LikeCount))
	}
	if v.DurationStr != "" {
		stats = append(stats, styles.Accent.Bold(true).Render("Duration: ")+styles.Subtitle.Render(v.DurationStr))
	}
	if v.PublishedAt != "" {
		stats = append(stats, styles.Accent.Bold(true).Render("Published: ")+styles.Subtitle.Render(v.PublishedAt))
	}
	b.WriteString(strings.Join(stats, "  │  "))
	b.WriteString("\n\n")

	// URL
	b.WriteString(styles.Subtitle.Render(v.URL))
	b.WriteString("\n\n")
	b.WriteString(sep)
	b.WriteString("\n\n")

	// Description
	if v.Description != "" {
		b.WriteString(contentStyle.Width(m.width - 2).Render(v.Description))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Dim.Render("[p] play  [d] download  [o] open in browser  [y] copy URL  [c] channel  [tab] comments  [esc] back"))

	return b.String()
}


func viewportKeyMap() viewport.KeyMap {
	return viewport.KeyMap{
		PageDown:     key.NewBinding(key.WithKeys("ctrl+f", "pgdown")),
		PageUp:       key.NewBinding(key.WithKeys("ctrl+b", "pgup")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u")),
		Up:           key.NewBinding(key.WithKeys("k", "up")),
		Down:         key.NewBinding(key.WithKeys("j", "down")),
	}
}
