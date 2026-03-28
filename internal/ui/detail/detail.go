package detail

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

var separatorStyle = lipgloss.NewStyle().Foreground(styles.DarkGray)

// VideoLoadedMsg carries the loaded video details.
type VideoLoadedMsg struct {
	Video *youtube.Video
	Err   error
}

// Model is the video detail view.
type Model struct {
	viewport viewport.Model
	spinner  spinner.Model
	video    *youtube.Video
	loading  bool
	width    int
	height   int
	client   youtube.Client
}

// New creates a new detail view model.
func New(client youtube.Client) Model {
	return Model{
		spinner: styles.NewSpinner(),
		client:  client,
	}
}

// SetSize updates the view dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.viewport.Width = w
	m.viewport.Height = h
	if m.video != nil {
		m.viewport.SetContent(m.renderDetail())
	}
}

// LoadVideo starts loading a video's details.
func (m *Model) LoadVideo(id string) tea.Cmd {
	m.loading = true
	m.video = nil
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
		if msg.Err != nil {
			m.viewport.SetContent(fmt.Sprintf("Error loading video: %v", msg.Err))
			return m, nil
		}
		m.video = msg.Video
		m.viewport = viewport.New(m.width, m.height)
		m.viewport.KeyMap = viewportKeyMap()
		m.viewport.SetContent(m.renderDetail())
		return m, nil

	case tea.KeyMsg:
		if !m.loading {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
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
		return m.spinner.View() + " Loading video details..."
	}
	return m.viewport.View()
}

func (m Model) renderDetail() string {
	v := m.video
	if v == nil {
		return ""
	}

	var b strings.Builder

	sep := separatorStyle.Render(strings.Repeat("─", m.width-2))

	b.WriteString(styles.Title.MarginBottom(1).Width(m.width - 2).Render(v.Title))
	b.WriteString("\n")

	channel := styles.Accent.Bold(true).Render(v.ChannelName)
	if v.SubscriberCount != "" {
		channel += styles.Subtitle.Render("  " + v.SubscriberCount)
	}
	b.WriteString(channel)
	b.WriteString("\n\n")

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

	b.WriteString(styles.Subtitle.Render(v.URL))
	b.WriteString("\n\n")
	b.WriteString(sep)
	b.WriteString("\n\n")

	if v.Description != "" {
		b.WriteString(styles.Subtitle.Width(m.width - 2).Render(v.Description))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(sep)
	b.WriteString("\n")
	b.WriteString(styles.Dim.Render("[p] play  [d] download  [o] open in browser  [y] copy URL  [esc] back"))

	return b.String()
}

func viewportKeyMap() viewport.KeyMap {
	return viewport.KeyMap{
		PageDown: key.NewBinding(key.WithKeys("ctrl+f", "pgdown")),
		PageUp:   key.NewBinding(key.WithKeys("ctrl+b", "pgup")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u")),
		Up:   key.NewBinding(key.WithKeys("k", "up")),
		Down: key.NewBinding(key.WithKeys("j", "down")),
	}
}
