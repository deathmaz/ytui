package app

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// Model is the root Bubble Tea model.
type Model struct {
	activeView View
	prevView   View
	width      int
	height     int
	keys       KeyMap
	help       help.Model
	search     search.Model
	ytClient   youtube.Client
}

// New creates a new root model with the given YouTube client.
func New(client youtube.Client) Model {
	h := help.New()
	h.ShortSeparator = "  "
	return Model{
		activeView: ViewSearch,
		keys:       DefaultKeyMap(),
		help:       h,
		search:     search.New(client),
		ytClient:   client,
	}
}

func (m Model) Init() tea.Cmd {
	return m.search.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		contentHeight := m.height - 4 // tabs + help
		m.search.SetSize(msg.Width, contentHeight)

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		case key.Matches(msg, m.keys.Feed):
			m.prevView = m.activeView
			m.activeView = ViewFeed
			return m, nil
		case key.Matches(msg, m.keys.Subs):
			m.prevView = m.activeView
			m.activeView = ViewSubs
			return m, nil
		case key.Matches(msg, m.keys.Search):
			m.prevView = m.activeView
			m.activeView = ViewSearch
			m.search.Focus()
			return m, nil
		case key.Matches(msg, m.keys.Back):
			if m.activeView != ViewFeed && m.activeView != ViewSubs && m.activeView != ViewSearch {
				m.activeView = m.prevView
				return m, nil
			}
		}

	case search.VideoSelectedMsg:
		// TODO: open detail view or play
		return m, nil
	}

	// Delegate to active view
	switch m.activeView {
	case ViewSearch:
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	tabs := m.renderTabs()
	content := m.renderContent()
	helpView := statusBarStyle.Render(m.help.View(m.keys))

	return lipgloss.JoinVertical(lipgloss.Left,
		tabs,
		content,
		helpView,
	)
}

func (m Model) renderTabs() string {
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

func (m Model) renderContent() string {
	switch m.activeView {
	case ViewSearch:
		return m.search.View()
	case ViewFeed:
		return m.renderPlaceholder("Feed - aggregated subscription videos")
	case ViewSubs:
		return m.renderPlaceholder("Subscriptions")
	case ViewDetail:
		return m.renderPlaceholder("Video Details")
	case ViewComments:
		return m.renderPlaceholder("Comments")
	}
	return ""
}

func (m Model) renderPlaceholder(label string) string {
	contentHeight := m.height - 4
	return placeholderStyle.
		Width(m.width).
		Height(contentHeight).
		Render(label)
}
