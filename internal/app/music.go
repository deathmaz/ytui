package app

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/config"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

type musicView int

const (
	musicViewSearch musicView = iota
)

type musicSearchResultMsg struct {
	Result *youtube.MusicSearchResult
	Err    error
}

// musicItem wraps a MusicItem for the list component.
type musicItem struct {
	item youtube.MusicItem
}

func (m musicItem) FilterValue() string { return m.item.Title }
func (m musicItem) Title() string       { return m.item.Title }
func (m musicItem) Description() string { return m.item.Subtitle }

// MusicModel is the root Bubble Tea model for music mode.
type MusicModel struct {
	activeView musicView
	width      int
	height     int
	keys       KeyMap
	help       help.Model
	client     *youtube.MusicClient
	cfg        *config.Config

	// Search
	searchInput    textinput.Model
	searchResults  list.Model
	searchSpinner  spinner.Model
	searching      bool
	searchFocused  bool // true = input focused, false = list focused
	query          string

	statusMsg string
	statusSeq int
}

// NewMusic creates a new root model for music mode.
func NewMusic(client *youtube.MusicClient, cfg *config.Config, opts Options) *MusicModel {
	h := help.New()
	h.ShortSeparator = "  "

	ti := textinput.New()
	ti.Placeholder = "Search YouTube Music..."
	ti.CharLimit = 256
	ti.Focus()

	sp := styles.NewSpinner()

	l := list.New(nil, musicDelegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.SetShowPagination(true)
	l.KeyMap.Quit = key.NewBinding()
	l.KeyMap.GoToStart = key.NewBinding(key.WithKeys("g", "home"))
	l.KeyMap.GoToEnd = key.NewBinding(key.WithKeys("G", "end"))

	m := &MusicModel{
		activeView:    musicViewSearch,
		keys:          DefaultKeyMap(),
		help:          h,
		client:        client,
		cfg:           cfg,
		searchInput:   ti,
		searchResults: l,
		searchSpinner: sp,
		searchFocused: true,
	}

	if opts.SearchQuery != "" {
		m.query = opts.SearchQuery
		m.searchInput.SetValue(opts.SearchQuery)
		m.searchInput.Blur()
		m.searchFocused = false
	}

	return m
}

func (m *MusicModel) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if m.query != "" {
		m.searching = true
		cmds = append(cmds, m.searchSpinner.Tick, m.searchCmd(m.query))
	}
	return tea.Batch(cmds...)
}

func (m *MusicModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.resizeViews()

	case tea.KeyMsg:
		if key.Matches(msg, m.keys.ForceQuit) {
			return m, tea.Quit
		}

		if m.searchFocused {
			switch {
			case msg.String() == "enter":
				query := m.searchInput.Value()
				if query == "" {
					return m, nil
				}
				m.query = query
				m.searching = true
				m.searchFocused = false
				m.searchInput.Blur()
				m.searchResults.SetItems(nil)
				m.searchResults.ResetSelected()
				return m, tea.Batch(m.searchSpinner.Tick, m.searchCmd(query))
			case msg.String() == "esc":
				m.searchFocused = false
				m.searchInput.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				return m, cmd
			}
		}

		// List focused
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Search), msg.String() == "/":
			m.searchFocused = true
			m.searchInput.Focus()
			return m, textinput.Blink
		case key.Matches(msg, m.keys.Play):
			return m, m.playSelected()
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		default:
			var cmd tea.Cmd
			m.searchResults, cmd = m.searchResults.Update(msg)
			cmds = append(cmds, cmd)
		}

	case musicSearchResultMsg:
		m.searching = false
		if msg.Err != nil {
			return m, m.setStatus("Search error: "+msg.Err.Error(), 5*time.Second)
		}
		var items []list.Item
		if msg.Result.TopResult != nil {
			items = append(items, musicItem{item: *msg.Result.TopResult})
		}
		for _, shelf := range msg.Result.Shelves {
			for _, item := range shelf.Items {
				items = append(items, musicItem{item: item})
			}
		}
		cmd := m.searchResults.SetItems(items)
		cmds = append(cmds, cmd)

	case musicPlayReadyMsg:
		if msg.err != nil {
			return m, m.setStatus("Play error: "+msg.err.Error(), 5*time.Second)
		}
		return m, playVideoCmd(msg.url, m.cfg.Player.Quality, m.cfg.Player.Command, m.cfg.Player.Args)

	case clearStatusMsg:
		if msg.seq == m.statusSeq {
			m.statusMsg = ""
			m.resizeViews()
		}

	case spinner.TickMsg:
		if m.searching {
			var cmd tea.Cmd
			m.searchSpinner, cmd = m.searchSpinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *MusicModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	tabs := m.renderTabs()
	content := m.renderContent()

	var statusLine string
	if m.statusMsg != "" {
		statusLine = styles.Accent.Render(m.statusMsg)
	}

	helpView := statusBarStyle.Render(m.help.View(m.keys))

	var sections []string
	sections = append(sections, tabs, content)
	if statusLine != "" {
		sections = append(sections, statusLine)
	}
	sections = append(sections, helpView)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m *MusicModel) renderTabs() string {
	label := "[3] Search"
	rendered := activeTabStyle.Render(label)
	bar := lipgloss.JoinHorizontal(lipgloss.Top, rendered)
	return tabSeparatorStyle.Width(m.width).Render(bar)
}

func (m *MusicModel) renderContent() string {
	inputView := lipgloss.NewStyle().Padding(0, 1).Width(m.width).Render(m.searchInput.View())

	if m.searching {
		return lipgloss.JoinVertical(lipgloss.Left,
			inputView,
			m.searchSpinner.View()+" Searching...",
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		inputView,
		m.searchResults.View(),
	)
}

func (m *MusicModel) resizeViews() {
	tabs := m.renderTabs()
	helpView := statusBarStyle.Render(m.help.View(m.keys))
	overhead := lipgloss.Height(tabs) + lipgloss.Height(helpView)
	if m.statusMsg != "" {
		overhead++
	}
	ch := m.height - overhead

	inputView := lipgloss.NewStyle().Padding(0, 1).Width(m.width).Render(m.searchInput.View())
	inputHeight := lipgloss.Height(inputView)
	m.searchResults.SetSize(m.width, ch-inputHeight)
	m.searchInput.Width = m.width - 4
}

func (m *MusicModel) searchCmd(query string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		result, err := client.Search(context.Background(), query)
		return musicSearchResultMsg{Result: result, Err: err}
	}
}

type musicPlayReadyMsg struct {
	url string
	err error
}

func (m *MusicModel) playSelected() tea.Cmd {
	item, ok := m.searchResults.SelectedItem().(musicItem)
	if !ok {
		return nil
	}
	it := item.item

	// Songs/videos have direct videoID
	if it.VideoID != "" {
		url := youtube.VideoURL(it.VideoID)
		return playVideoCmd(url, m.cfg.Player.Quality, m.cfg.Player.Command, m.cfg.Player.Args)
	}

	// Albums/playlists need browsing to get a playable URL
	if it.BrowseID != "" {
		browseID := it.BrowseID
		client := m.client
		return tea.Batch(
			m.setStatus("Loading tracks...", 10*time.Second),
			func() tea.Msg {
				tracks, playlistID, err := client.GetAlbumTracks(context.Background(), browseID)
				if err != nil {
					return musicPlayReadyMsg{err: err}
				}
				if playlistID != "" {
					// Play the whole album/playlist via playlist URL
					return musicPlayReadyMsg{url: youtube.PlaylistURL(playlistID)}
				}
				if len(tracks) > 0 && tracks[0].VideoID != "" {
					return musicPlayReadyMsg{url: youtube.VideoURL(tracks[0].VideoID)}
				}
				return musicPlayReadyMsg{err: fmt.Errorf("no playable tracks found")}
			},
		)
	}

	return m.setStatus("Cannot play this item", 3*time.Second)
}

func (m *MusicModel) setStatus(msg string, clearAfter time.Duration) tea.Cmd {
	m.statusSeq++
	m.statusMsg = msg
	m.resizeViews()
	seq := m.statusSeq
	return tea.Tick(clearAfter, func(time.Time) tea.Msg {
		return clearStatusMsg{seq: seq}
	})
}
