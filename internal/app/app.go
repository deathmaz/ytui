package app

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/player"
	"github.com/deathmaz/ytui/internal/ui/detail"
	"github.com/deathmaz/ytui/internal/ui/picker"
	"github.com/deathmaz/ytui/internal/ui/search"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

var (
	tabStyle = lipgloss.NewStyle().
			Padding(0, 2)

	activeTabStyle = tabStyle.
			Bold(true).
			Foreground(styles.Red)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(styles.DimGray)

	tabSeparatorStyle = lipgloss.NewStyle().
				BorderBottom(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(styles.DarkGray)

	placeholderStyle = lipgloss.NewStyle().
				Align(lipgloss.Center, lipgloss.Center).
				Foreground(styles.MidGray)
)

type playerErrorMsg struct{ err error }

// Model is the root Bubble Tea model.
type Model struct {
	activeView View
	prevView   View
	width      int
	height     int
	keys       KeyMap
	help       help.Model
	search     search.Model
	detail     detail.Model
	ytClient   youtube.Client
	picker     picker.Model
	playerCmd  string

	pendingVideoURL string
}

// New creates a new root model with the given YouTube client.
func New(client youtube.Client) *Model {
	h := help.New()
	h.ShortSeparator = "  "
	return &Model{
		activeView: ViewSearch,
		keys:       DefaultKeyMap(),
		help:       h,
		search:     search.New(client),
		detail:     detail.New(client),
		ytClient:   client,
		picker:     picker.New(),
		playerCmd:  "mpv",
	}
}

func (m *Model) Init() tea.Cmd {
	return m.search.Init()
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.search.SetSize(msg.Width, m.contentHeight())
		m.detail.SetSize(msg.Width, m.contentHeight())

	case tea.KeyMsg:
		// Quality picker takes priority when active
		if m.picker.IsActive() {
			var cmd tea.Cmd
			m.picker, cmd = m.picker.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}

		// ctrl+c always quits, regardless of input focus
		if key.Matches(msg, m.keys.ForceQuit) {
			return m, tea.Quit
		}

		inputHasFocus := m.activeView == ViewSearch && m.search.InputFocused()

		// Esc handling
		if key.Matches(msg, m.keys.Back) && !inputHasFocus {
			if m.activeView != ViewFeed && m.activeView != ViewSubs && m.activeView != ViewSearch {
				m.activeView = m.prevView
				return m, nil
			}
		}

		// Skip single-key global bindings when text input has focus
		if !inputHasFocus {
			switch {
			case key.Matches(msg, m.keys.Quit):
				return m, tea.Quit
			case key.Matches(msg, m.keys.Help):
				m.help.ShowAll = !m.help.ShowAll
				return m, nil
			case key.Matches(msg, m.keys.Feed):
				m.switchTo(ViewFeed)
				return m, nil
			case key.Matches(msg, m.keys.Subs):
				m.switchTo(ViewSubs)
				return m, nil
			case key.Matches(msg, m.keys.Search):
				m.switchTo(ViewSearch)
				m.search.Focus()
				return m, nil
			case key.Matches(msg, m.keys.Detail):
				return m, m.openDetail()
			case key.Matches(msg, m.keys.Play):
				m.openQualityPicker()
				return m, nil
			}
		}

	case search.VideoSelectedMsg:
		// Enter on a video opens detail view
		m.switchTo(ViewDetail)
		return m, m.detail.LoadVideo(msg.Video.ID)

	case picker.SelectedMsg:
		if m.pendingVideoURL != "" {
			url := m.pendingVideoURL
			format := msg.Format.ID
			cmd := m.playerCmd
			m.pendingVideoURL = ""
			return m, playVideoCmd(url, format, cmd)
		}
		return m, nil

	case picker.CancelledMsg:
		m.pendingVideoURL = ""
		return m, nil

	case playerErrorMsg:
		// TODO: show error in status bar
		_ = msg.err
	}

	// Delegate to active view
	switch m.activeView {
	case ViewSearch:
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		cmds = append(cmds, cmd)
	case ViewDetail:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	if m.picker.IsActive() {
		return m.picker.View()
	}

	tabs := m.renderTabs()
	content := m.renderContent()
	helpView := statusBarStyle.Render(m.help.View(m.keys))

	return lipgloss.JoinVertical(lipgloss.Left, tabs, content, helpView)
}

func (m *Model) switchTo(v View) {
	m.prevView = m.activeView
	m.activeView = v
}

func (m *Model) selectedVideo() *youtube.Video {
	switch m.activeView {
	case ViewSearch:
		if v, ok := m.search.SelectedVideo(); ok {
			return &v
		}
	case ViewDetail:
		return m.detail.Video()
	}
	return nil
}

func (m *Model) openDetail() tea.Cmd {
	v := m.selectedVideo()
	if v == nil {
		return nil
	}
	m.switchTo(ViewDetail)
	return m.detail.LoadVideo(v.ID)
}

func (m *Model) openQualityPicker() {
	v := m.selectedVideo()
	if v == nil {
		return
	}
	m.pendingVideoURL = v.URL
	m.picker.Show(player.CommonFormats(), m.width, m.height)
}

func playVideoCmd(url, format, playerCmd string) tea.Cmd {
	return func() tea.Msg {
		if err := player.Play(url, format, playerCmd); err != nil {
			return playerErrorMsg{err: err}
		}
		return nil
	}
}

func (m *Model) contentHeight() int {
	return m.height - 4 // tabs + help
}

func (m *Model) renderTabs() string {
	tabs := []struct {
		label string
		view  View
	}{
		{"[1] Feed", ViewFeed},
		{"[2] Subs", ViewSubs},
		{"[3] Search", ViewSearch},
	}

	var rendered []string
	for _, t := range tabs {
		style := tabStyle
		if t.view == m.activeView {
			style = activeTabStyle
		}
		rendered = append(rendered, style.Render(t.label))
	}

	bar := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
	return tabSeparatorStyle.Width(m.width).Render(bar)
}

func (m *Model) renderContent() string {
	switch m.activeView {
	case ViewSearch:
		return m.search.View()
	case ViewDetail:
		return m.detail.View()
	case ViewFeed:
		return m.renderPlaceholder("Feed - aggregated subscription videos")
	case ViewSubs:
		return m.renderPlaceholder("Subscriptions")
	case ViewComments:
		return m.renderPlaceholder("Comments")
	}
	return ""
}

func (m *Model) renderPlaceholder(label string) string {
	return placeholderStyle.
		Width(m.width).
		Height(m.contentHeight()).
		Render(label)
}
