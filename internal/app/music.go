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
	"github.com/deathmaz/ytui/internal/auth"
	"github.com/deathmaz/ytui/internal/config"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

type musicKeyMap struct{}

func (k musicKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "play")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next section")),
		key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "load all")),
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "auth")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

func (k musicKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

const maxMusicTabs = 6

type musicFixedView int

const (
	musicViewHome    musicFixedView = 0
	musicViewLibrary musicFixedView = 1
	musicViewSearch  musicFixedView = 2
)

type musicTabKind int

const (
	musicTabArtist musicTabKind = iota
	musicTabAlbum
)

type subTab struct {
	title        string
	list         list.Model
	continuation string // for library pagination
}

type musicTab struct {
	kind         musicTabKind
	title        string
	browseID     string
	// Artist page
	artistPage   *youtube.MusicArtistPage
	artistSubs   []subTab
	activeSubTab int
	// Album page
	albumPage    *youtube.MusicAlbumPage
	albumList    list.Model
	loaded       bool
}

type musicHomeLoadedMsg struct {
	Shelves []youtube.MusicShelf
	Err     error
}

type musicLibrarySectionMsg struct {
	Index        int
	Items        []youtube.MusicItem
	Continuation string
	Err          error
}

type musicSearchResultMsg struct {
	Result *youtube.MusicSearchResult
	Err    error
}

type musicArtistLoadedMsg struct {
	BrowseID string
	Artist   *youtube.MusicArtistPage
	Err      error
}

type musicAlbumLoadedMsg struct {
	BrowseID string
	Album    *youtube.MusicAlbumPage
	Err      error
}

type musicMoreLoadedMsg struct {
	SubTabIdx int
	Items     []youtube.MusicItem
	Err       error
}

type musicAuthSuccessMsg struct{ client *youtube.MusicClient }
type musicAuthFailedMsg struct{ err error }

// musicItem wraps a MusicItem for the list component.
type musicItem struct {
	item youtube.MusicItem
}

func (m musicItem) FilterValue() string { return m.item.Title }
func (m musicItem) Title() string       { return m.item.Title }
func (m musicItem) Description() string { return m.item.Subtitle }

// MusicModel is the root Bubble Tea model for music mode.
type MusicModel struct {
	onFixedView  bool // true = fixed view (home/library/search), false = dynamic tab
	width        int
	height       int
	keys         KeyMap
	help         help.Model
	client       *youtube.MusicClient
	cfg          *config.Config

	// Fixed views
	activeFixed   musicFixedView
	searchInput   textinput.Model
	searchResults list.Model
	searchSpinner spinner.Model
	searching     bool
	searchFocused bool
	query         string

	// Home
	homeSubs   []subTab
	homeSubIdx int
	homeLoaded  bool
	homeLoading bool

	// Library
	librarySubs    []subTab
	librarySubIdx  int
	libraryLoaded  bool
	libraryLoading bool
	libraryPending int // number of sections still loading

	// Dynamic tabs
	tabs         []musicTab
	activeTabIdx int
	pageLoading  bool

	authenticating bool
	statusMsg      string
	statusSeq      int
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

	l := shared.NewList(musicDelegate{})

	m := &MusicModel{
		onFixedView:   true,
		activeFixed:   musicViewSearch,
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
	if m.cfg.Auth.AuthOnStartup {
		cmds = append(cmds, m.authenticate())
	}
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

		// Global keys (not in search input)
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		case key.Matches(msg, m.keys.Back):
			if !m.onFixedView {
				// Close current tab
				m.closeActiveTab()
				return m, nil
			}
		case key.Matches(msg, m.keys.Play):
			return m, m.playSelected()
		case key.Matches(msg, m.keys.Auth):
			return m, m.authenticate()
		case key.Matches(msg, m.keys.Search), msg.String() == "/":
			m.onFixedView = true
			m.activeFixed = musicViewSearch
			m.searchFocused = true
			m.searchInput.Focus()
			return m, textinput.Blink
		case msg.String() == "enter":
			return m, m.openSelected()
		}

		// Tab keys: 1=Home, 2=Library, 3=Search, 4+=dynamic tabs
		if k := msg.String(); len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
			idx := int(k[0] - '1')
			if idx <= 2 {
				m.onFixedView = true
				m.activeFixed = musicFixedView(idx)
				m.searchFocused = false
				m.searchInput.Blur()
				switch m.activeFixed {
				case musicViewHome:
					return m, m.loadHome()
				case musicViewLibrary:
					return m, m.loadLibrary()
				}
				return m, nil
			}
			tabIdx := idx - 3
			if tabIdx < len(m.tabs) {
				m.onFixedView = false
				m.activeTabIdx = tabIdx
				return m, nil
			}
		}

		// Sub-tab navigation for artist pages
		if !m.onFixedView {
			if tab := m.activeTab(); tab != nil && tab.kind == musicTabArtist && tab.loaded {
				if msg.String() == "tab" {
					if tab.activeSubTab < len(tab.artistSubs)-1 {
						tab.activeSubTab++
						m.resizeViews()
					}
					return m, nil
				}
				if msg.String() == "shift+tab" {
					if tab.activeSubTab > 0 {
						tab.activeSubTab--
						m.resizeViews()
					}
					return m, nil
				}
				if msg.String() == "L" {
					return m, m.loadMoreForSubTab(tab)
				}
			}
		}

		// Sub-tab navigation for home and library views
		if m.onFixedView && m.activeFixed == musicViewHome && m.homeLoaded {
			if msg.String() == "tab" {
				if m.homeSubIdx < len(m.homeSubs)-1 {
					m.homeSubIdx++
				}
				return m, nil
			}
			if msg.String() == "shift+tab" {
				if m.homeSubIdx > 0 {
					m.homeSubIdx--
				}
				return m, nil
			}
		}
		if m.onFixedView && m.activeFixed == musicViewLibrary && m.libraryLoaded {
			if msg.String() == "tab" {
				if m.librarySubIdx < len(m.librarySubs)-1 {
					m.librarySubIdx++
				}
				return m, nil
			}
			if msg.String() == "shift+tab" {
				if m.librarySubIdx > 0 {
					m.librarySubIdx--
				}
				return m, nil
			}
			if msg.String() == "L" {
				return m, m.loadMoreLibrary()
			}
		}

		// Delegate to active list
		if m.onFixedView {
			switch m.activeFixed {
			case musicViewHome:
				if m.homeLoaded && m.homeSubIdx < len(m.homeSubs) {
					var cmd tea.Cmd
					m.homeSubs[m.homeSubIdx].list, cmd = m.homeSubs[m.homeSubIdx].list.Update(msg)
					cmds = append(cmds, cmd)
				}
			case musicViewLibrary:
				if m.libraryLoaded && m.librarySubIdx < len(m.librarySubs) {
					var cmd tea.Cmd
					m.librarySubs[m.librarySubIdx].list, cmd = m.librarySubs[m.librarySubIdx].list.Update(msg)
					cmds = append(cmds, cmd)
				}
			case musicViewSearch:
				var cmd tea.Cmd
				m.searchResults, cmd = m.searchResults.Update(msg)
				cmds = append(cmds, cmd)
			}
		} else if tab := m.activeTab(); tab != nil {
			switch tab.kind {
			case musicTabArtist:
				if tab.loaded && tab.activeSubTab < len(tab.artistSubs) {
					var cmd tea.Cmd
					tab.artistSubs[tab.activeSubTab].list, cmd = tab.artistSubs[tab.activeSubTab].list.Update(msg)
					cmds = append(cmds, cmd)
				}
			case musicTabAlbum:
				var cmd tea.Cmd
				tab.albumList, cmd = tab.albumList.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

	case musicHomeLoadedMsg:
		m.homeLoading = false
		if msg.Err != nil {
			return m, m.setStatus("Home error: "+msg.Err.Error(), 5*time.Second)
		}
		m.homeLoaded = true
		m.homeSubs = shelvesToSubTabs(msg.Shelves)
		m.homeSubIdx = 0
		return m, nil

	case musicLibrarySectionMsg:
		if msg.Err != nil {
			m.libraryPending--
			if m.libraryPending <= 0 {
				m.libraryLoading = false
				m.libraryLoaded = true
			}
			return m, m.setStatus("Library error: "+msg.Err.Error(), 5*time.Second)
		}
		if msg.Index < len(m.librarySubs) {
			sub := &m.librarySubs[msg.Index]
			// Append to existing items (for continuation loads)
			existing := sub.list.Items()
			for _, it := range msg.Items {
				existing = append(existing, musicItem{item: it})
			}
			sub.list.SetItems(existing)
			sub.continuation = msg.Continuation
		}
		if m.libraryPending > 0 {
			m.libraryPending--
		}
		if m.libraryPending <= 0 {
			m.libraryLoading = false
			m.libraryLoaded = true
			m.resizeViews()
		}
		m.statusMsg = ""
		return m, nil

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

	case musicArtistLoadedMsg:
		m.pageLoading = false
		if msg.Err != nil {
			return m, m.setStatus("Error: "+msg.Err.Error(), 5*time.Second)
		}
		for i := range m.tabs {
			tab := &m.tabs[i]
			if tab.browseID == msg.BrowseID && tab.kind == musicTabArtist && !tab.loaded {
				tab.artistPage = msg.Artist
				tab.artistSubs = m.buildArtistSubTabs(msg.Artist)
				tab.loaded = true
				m.resizeViews()
				break
			}
		}
		return m, nil

	case musicAlbumLoadedMsg:
		m.pageLoading = false
		if msg.Err != nil {
			return m, m.setStatus("Error: "+msg.Err.Error(), 5*time.Second)
		}
		for i := range m.tabs {
			tab := &m.tabs[i]
			if tab.browseID == msg.BrowseID && tab.kind == musicTabAlbum && !tab.loaded {
				tab.albumPage = msg.Album
				tab.albumList = m.buildAlbumList(msg.Album)
				tab.loaded = true
				m.resizeViews()
				break
			}
		}
		return m, nil

	case musicMoreLoadedMsg:
		m.statusMsg = ""
		if msg.Err != nil {
			return m, m.setStatus("Error loading more: "+msg.Err.Error(), 5*time.Second)
		}
		if tab := m.activeTab(); tab != nil && tab.kind == musicTabArtist {
			if msg.SubTabIdx < len(tab.artistSubs) {
				sub := &tab.artistSubs[msg.SubTabIdx]
				var items []list.Item
				for _, it := range msg.Items {
					items = append(items, musicItem{item: it})
				}
				sub.list.SetItems(items)
				// Clear the more endpoint since we loaded all
				for i := range tab.artistPage.Shelves {
					if tab.artistPage.Shelves[i].Title == sub.title {
						tab.artistPage.Shelves[i].MoreBrowseID = ""
						tab.artistPage.Shelves[i].MoreParams = ""
						break
					}
				}
			}
		}
		return m, nil

	case musicPlayReadyMsg:
		if msg.err != nil {
			return m, m.setStatus("Play error: "+msg.err.Error(), 5*time.Second)
		}
		return m, playVideoCmd(msg.url, m.cfg.Player.Quality, m.cfg.Player.Command, m.cfg.Player.Args)

	case musicAuthSuccessMsg:
		m.authenticating = false
		m.client = msg.client
		// Reset home/library so they reload with auth
		m.homeLoaded = false
		m.homeLoading = false
		m.libraryLoaded = false
		m.libraryLoading = false
		m.librarySubs = nil
		m.librarySubIdx = 0
		m.resizeViews()
		var reloadCmd tea.Cmd
		switch m.activeFixed {
		case musicViewHome:
			reloadCmd = m.loadHome()
		case musicViewLibrary:
			reloadCmd = m.loadLibrary()
		}
		return m, tea.Batch(m.setStatus("Authenticated via "+m.cfg.Auth.Browser, 3*time.Second), reloadCmd)

	case musicAuthFailedMsg:
		m.authenticating = false
		return m, m.setStatus("Auth failed: "+msg.err.Error(), 5*time.Second)

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

	helpView := statusBarStyle.Render(m.help.View(musicKeyMap{}))

	var sections []string
	sections = append(sections, tabs, content)
	if statusLine != "" {
		sections = append(sections, statusLine)
	}
	sections = append(sections, helpView)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m *MusicModel) loadHome() tea.Cmd {
	if m.homeLoaded || m.homeLoading {
		return nil
	}
	m.homeLoading = true
	client := m.client
	return tea.Batch(m.searchSpinner.Tick, func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = musicHomeLoadedMsg{Err: fmt.Errorf("panic: %v", r)}
			}
		}()
		shelves, err := client.GetHome(context.Background())
		return musicHomeLoadedMsg{Shelves: shelves, Err: err}
	})
}

func (m *MusicModel) loadLibrary() tea.Cmd {
	if m.libraryLoaded || m.libraryLoading {
		return nil
	}
	if !m.client.IsAuthenticated() {
		return m.setStatus("Press 'a' to authenticate first", 3*time.Second)
	}
	m.libraryLoading = true

	// Initialize sub-tabs with empty lists
	sections := youtube.LibrarySections
	m.librarySubs = make([]subTab, len(sections))
	for i, sec := range sections {
		m.librarySubs[i] = subTab{title: sec.Title, list: shared.NewList(musicDelegate{})}
	}
	m.librarySubIdx = 0
	m.libraryPending = len(sections)

	// Fetch all sections concurrently
	client := m.client
	var cmds []tea.Cmd
	cmds = append(cmds, m.searchSpinner.Tick)
	for i, sec := range sections {
		idx := i
		browseID := sec.BrowseID
		cmds = append(cmds, func() (msg tea.Msg) {
			defer func() {
				if r := recover(); r != nil {
					msg = musicLibrarySectionMsg{Index: idx, Err: fmt.Errorf("panic: %v", r)}
				}
			}()
			res, err := client.GetLibrarySection(context.Background(), browseID)
			if err != nil {
				return musicLibrarySectionMsg{Index: idx, Err: err}
			}
			return musicLibrarySectionMsg{Index: idx, Items: res.Items, Continuation: res.Continuation}
		})
	}
	return tea.Batch(cmds...)
}

func shelvesToSubTabs(shelves []youtube.MusicShelf) []subTab {
	var subs []subTab
	for _, shelf := range shelves {
		if len(shelf.Items) == 0 {
			continue
		}
		l := shared.NewList(musicDelegate{})
		var items []list.Item
		for _, it := range shelf.Items {
			items = append(items, musicItem{item: it})
		}
		l.SetItems(items)
		subs = append(subs, subTab{title: shelf.Title, list: l})
	}
	return subs
}

func (m *MusicModel) loadMoreForSubTab(tab *musicTab) tea.Cmd {
	if tab.activeSubTab >= len(tab.artistSubs) || tab.artistPage == nil {
		return nil
	}
	sub := tab.artistSubs[tab.activeSubTab]
	// Find the matching shelf to get the more endpoint
	var shelf *youtube.MusicShelf
	for i := range tab.artistPage.Shelves {
		if tab.artistPage.Shelves[i].Title == sub.title {
			shelf = &tab.artistPage.Shelves[i]
			break
		}
	}
	if shelf == nil || shelf.MoreBrowseID == "" {
		return m.setStatus("All items loaded", 2*time.Second)
	}

	browseID := shelf.MoreBrowseID
	params := shelf.MoreParams
	subIdx := tab.activeSubTab
	client := m.client
	return tea.Batch(
		m.setStatus("Loading all "+sub.title+"...", 10*time.Second),
		func() tea.Msg {
			items, err := client.BrowseMore(context.Background(), browseID, params)
			return musicMoreLoadedMsg{SubTabIdx: subIdx, Items: items, Err: err}
		},
	)
}

func (m *MusicModel) activeTab() *musicTab {
	if m.onFixedView || m.activeTabIdx >= len(m.tabs) {
		return nil
	}
	return &m.tabs[m.activeTabIdx]
}

func (m *MusicModel) closeActiveTab() {
	if m.onFixedView || len(m.tabs) == 0 {
		return
	}
	m.tabs = append(m.tabs[:m.activeTabIdx], m.tabs[m.activeTabIdx+1:]...)
	if len(m.tabs) == 0 {
		m.onFixedView = true
	} else if m.activeTabIdx >= len(m.tabs) {
		m.activeTabIdx = len(m.tabs) - 1
	}
}

func (m *MusicModel) openSelected() tea.Cmd {
	it := m.selectedMusicItem()
	if it == nil {
		return nil
	}
	// Album tracks: Enter plays
	if !m.onFixedView {
		if tab := m.activeTab(); tab != nil && tab.kind == musicTabAlbum {
			return m.playSelected()
		}
	}
	return m.openMusicItem(*it)
}

func (m *MusicModel) openMusicItem(it youtube.MusicItem) tea.Cmd {
	switch it.Type {
	case youtube.MusicArtist:
		if it.BrowseID == "" {
			return nil
		}
		return m.openTab(musicTabArtist, it.Title, it.BrowseID)
	case youtube.MusicAlbum:
		if it.BrowseID == "" {
			return nil
		}
		return m.openTab(musicTabAlbum, it.Title, it.BrowseID)
	case youtube.MusicSong, youtube.MusicVideo:
		if it.VideoID != "" {
			return playVideoCmd(youtube.VideoURL(it.VideoID), m.cfg.Player.Quality, m.cfg.Player.Command, m.cfg.Player.Args)
		}
	case youtube.MusicPlaylist:
		if it.BrowseID != "" {
			return m.openTab(musicTabAlbum, it.Title, it.BrowseID)
		}
	}
	return nil
}

func (m *MusicModel) openTab(kind musicTabKind, title, browseID string) tea.Cmd {
	// Check if already open
	for i, tab := range m.tabs {
		if tab.browseID == browseID {
			m.onFixedView = false
			m.activeTabIdx = i
			return nil
		}
	}
	if len(m.tabs) >= maxMusicTabs {
		return m.setStatus("Max tabs reached (close one with Esc)", 3*time.Second)
	}

	m.tabs = append(m.tabs, musicTab{
		kind:     kind,
		title:    title,
		browseID: browseID,
	})
	m.activeTabIdx = len(m.tabs) - 1
	m.onFixedView = false
	m.pageLoading = true

	client := m.client
	switch kind {
	case musicTabArtist:
		return tea.Batch(m.searchSpinner.Tick, func() tea.Msg {
			artist, err := client.GetArtist(context.Background(), browseID)
			return musicArtistLoadedMsg{BrowseID: browseID, Artist: artist, Err: err}
		})
	case musicTabAlbum:
		return tea.Batch(m.searchSpinner.Tick, func() tea.Msg {
			album, err := client.GetAlbum(context.Background(), browseID)
			return musicAlbumLoadedMsg{BrowseID: browseID, Album: album, Err: err}
		})
	}
	return nil
}

// buildArtistSubTabs is an alias for shelvesToSubTabs using artist page shelves.
func (m *MusicModel) buildArtistSubTabs(artist *youtube.MusicArtistPage) []subTab {
	return shelvesToSubTabs(artist.Shelves)
}

func (m *MusicModel) buildAlbumList(album *youtube.MusicAlbumPage) list.Model {
	l := shared.NewList(musicTrackDelegate{})
	var items []list.Item
	for i, track := range album.Tracks {
		track.Subtitle = fmt.Sprintf("%d. %s", i+1, track.Subtitle)
		items = append(items, musicItem{item: track})
	}
	l.SetItems(items)
	return l
}

func (m *MusicModel) renderTabs() string {
	var rendered []string

	// Fixed tabs
	fixedTabs := []struct {
		label string
		view  musicFixedView
	}{
		{"[1] Home", musicViewHome},
		{"[2] Library", musicViewLibrary},
		{"[3] Search", musicViewSearch},
	}
	for _, ft := range fixedTabs {
		style := tabStyle
		if m.onFixedView && m.activeFixed == ft.view {
			style = activeTabStyle
		}
		rendered = append(rendered, style.Render(ft.label))
	}

	// Dynamic tabs
	for i, tab := range m.tabs {
		icon := "♫"
		if tab.kind == musicTabAlbum {
			icon = "◉"
		}
		label := fmt.Sprintf("[%d] %s", i+4, shared.Truncate(icon+" "+tab.title, 22))
		style := tabStyle
		if !m.onFixedView && m.activeTabIdx == i {
			style = activeTabStyle
		}
		rendered = append(rendered, style.Render(label))
	}

	bar := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
	return tabSeparatorStyle.Width(m.width).Render(bar)
}

func (m *MusicModel) renderContent() string {
	if m.pageLoading {
		return m.searchSpinner.View() + " Loading..."
	}

	if m.onFixedView {
		switch m.activeFixed {
		case musicViewHome:
			return m.renderHome()
		case musicViewLibrary:
			return m.renderLibrary()
		case musicViewSearch:
			return m.renderSearch()
		}
		return ""
	}

	if tab := m.activeTab(); tab != nil {
		if !tab.loaded {
			return m.searchSpinner.View() + " Loading..."
		}
		switch tab.kind {
		case musicTabArtist:
			return m.renderArtistPage(tab)
		case musicTabAlbum:
			return m.renderAlbumPage(tab)
		}
	}
	return ""
}

func (m *MusicModel) renderHome() string {
	if m.homeLoading {
		return m.searchSpinner.View() + " Loading home..."
	}
	if !m.homeLoaded || len(m.homeSubs) == 0 {
		return styles.Dim.Render("Press 1 to load home feed")
	}

	subBar := renderSubTabBar(m.homeSubs, m.homeSubIdx)

	ch := m.contentHeight()
	overhead := lipgloss.Height(subBar)
	if m.homeSubIdx < len(m.homeSubs) {
		m.homeSubs[m.homeSubIdx].list.SetSize(m.width, ch-overhead)
	}

	activeList := ""
	if m.homeSubIdx < len(m.homeSubs) {
		activeList = m.homeSubs[m.homeSubIdx].list.View()
	}

	return lipgloss.JoinVertical(lipgloss.Left, subBar, activeList)
}

func (m *MusicModel) renderLibrary() string {
	if m.libraryLoading {
		return m.searchSpinner.View() + " Loading library..."
	}
	if !m.libraryLoaded || len(m.librarySubs) == 0 {
		return styles.Dim.Render("Press 'a' to authenticate, then press 2 for Library")
	}

	subBar := renderSubTabBar(m.librarySubs, m.librarySubIdx)

	var hint string
	if m.librarySubIdx < len(m.librarySubs) && m.librarySubs[m.librarySubIdx].continuation != "" {
		hint = styles.Dim.Render("  Press L to load more")
	}

	ch := m.contentHeight()
	overhead := lipgloss.Height(subBar)
	if hint != "" {
		overhead += lipgloss.Height(hint)
	}
	if m.librarySubIdx < len(m.librarySubs) {
		m.librarySubs[m.librarySubIdx].list.SetSize(m.width, ch-overhead)
	}

	activeList := ""
	if m.librarySubIdx < len(m.librarySubs) {
		activeList = m.librarySubs[m.librarySubIdx].list.View()
	}

	var sections []string
	sections = append(sections, subBar, activeList)
	if hint != "" {
		sections = append(sections, hint)
	}
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m *MusicModel) renderSearch() string {
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

var (
	subTabStyle       = lipgloss.NewStyle().Padding(0, 1).Foreground(styles.DimGray)
	activeSubTabStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(styles.Cyan)
)

func renderSubTabBar(subs []subTab, activeIdx int) string {
	var labels []string
	for i, sub := range subs {
		style := subTabStyle
		if i == activeIdx {
			style = activeSubTabStyle
		}
		labels = append(labels, style.Render(sub.title))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, labels...)
}

func (m *MusicModel) renderArtistPage(tab *musicTab) string {
	if len(tab.artistSubs) == 0 {
		return styles.Dim.Render("No content")
	}

	subBar := renderSubTabBar(tab.artistSubs, tab.activeSubTab)

	// Check if more items are available
	var hint string
	if tab.artistPage != nil && tab.activeSubTab < len(tab.artistSubs) {
		subTitle := tab.artistSubs[tab.activeSubTab].title
		for _, shelf := range tab.artistPage.Shelves {
			if shelf.Title == subTitle && shelf.MoreBrowseID != "" {
				hint = styles.Dim.Render("  Press L to load all " + subTitle)
				break
			}
		}
	}

	// Measure overhead and resize list to fit exactly
	overhead := lipgloss.Height(subBar)
	if hint != "" {
		overhead += lipgloss.Height(hint)
	}
	ch := m.contentHeight()
	if tab.activeSubTab < len(tab.artistSubs) {
		tab.artistSubs[tab.activeSubTab].list.SetSize(m.width, ch-overhead)
	}

	activeList := ""
	if tab.activeSubTab < len(tab.artistSubs) {
		activeList = tab.artistSubs[tab.activeSubTab].list.View()
	}

	var sections []string
	sections = append(sections, subBar, activeList)
	if hint != "" {
		sections = append(sections, hint)
	}
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m *MusicModel) renderAlbumPage(tab *musicTab) string {
	if tab.albumPage == nil {
		return ""
	}
	header := styles.Title.Render(tab.albumPage.Title)
	if tab.albumPage.Artist != "" {
		header += "  " + styles.Subtitle.Render(tab.albumPage.Artist)
	}
	if tab.albumPage.Year != "" {
		header += "  " + styles.Dim.Render(tab.albumPage.Year)
	}

	ch := m.contentHeight()
	tab.albumList.SetSize(m.width, ch-lipgloss.Height(header))
	return lipgloss.JoinVertical(lipgloss.Left, header, tab.albumList.View())
}

func (m *MusicModel) contentHeight() int {
	tabs := m.renderTabs()
	helpView := statusBarStyle.Render(m.help.View(musicKeyMap{}))
	overhead := lipgloss.Height(tabs) + lipgloss.Height(helpView)
	if m.statusMsg != "" {
		overhead++
	}
	h := m.height - overhead
	if h < 1 {
		h = 1
	}
	return h
}

func (m *MusicModel) resizeViews() {
	if m.width == 0 {
		return
	}
	ch := m.contentHeight()
	inputView := lipgloss.NewStyle().Padding(0, 1).Width(m.width).Render(m.searchInput.View())
	inputHeight := lipgloss.Height(inputView)
	m.searchResults.SetSize(m.width, ch-inputHeight)
	m.searchInput.Width = m.width - 4
	// Artist/album lists are sized dynamically during rendering
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

func (m *MusicModel) selectedMusicItem() *youtube.MusicItem {
	var sel list.Item
	if m.onFixedView {
		switch m.activeFixed {
		case musicViewHome:
			if m.homeLoaded && m.homeSubIdx < len(m.homeSubs) {
				sel = m.homeSubs[m.homeSubIdx].list.SelectedItem()
			}
		case musicViewLibrary:
			if m.libraryLoaded && m.librarySubIdx < len(m.librarySubs) {
				sel = m.librarySubs[m.librarySubIdx].list.SelectedItem()
			}
		case musicViewSearch:
			sel = m.searchResults.SelectedItem()
		}
	} else if tab := m.activeTab(); tab != nil {
		switch tab.kind {
		case musicTabArtist:
			if tab.loaded && tab.activeSubTab < len(tab.artistSubs) {
				sel = tab.artistSubs[tab.activeSubTab].list.SelectedItem()
			}
		case musicTabAlbum:
			sel = tab.albumList.SelectedItem()
		}
	}
	if mi, ok := sel.(musicItem); ok {
		return &mi.item
	}
	return nil
}

func (m *MusicModel) playSelected() tea.Cmd {
	ptr := m.selectedMusicItem()
	if ptr == nil {
		return nil
	}
	it := *ptr

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

func (m *MusicModel) loadMoreLibrary() tea.Cmd {
	if m.librarySubIdx >= len(m.librarySubs) {
		return nil
	}
	sub := &m.librarySubs[m.librarySubIdx]
	if sub.continuation == "" {
		return m.setStatus("All items loaded", 2*time.Second)
	}
	cont := sub.continuation
	idx := m.librarySubIdx
	client := m.client
	sub.continuation = "" // prevent double-load
	return tea.Batch(
		m.setStatus("Loading more "+sub.title+"...", 10*time.Second),
		func() (msg tea.Msg) {
			defer func() {
				if r := recover(); r != nil {
					msg = musicLibrarySectionMsg{Index: idx, Err: fmt.Errorf("panic: %v", r)}
				}
			}()
			res, err := client.GetLibraryContinuation(context.Background(), cont)
			if err != nil {
				return musicLibrarySectionMsg{Index: idx, Err: err}
			}
			return musicLibrarySectionMsg{Index: idx, Items: res.Items, Continuation: res.Continuation}
		},
	)
}

func (m *MusicModel) authenticate() tea.Cmd {
	if m.authenticating {
		return nil
	}
	if m.client.IsAuthenticated() {
		return m.setStatus("Already authenticated", 3*time.Second)
	}
	m.authenticating = true
	m.statusMsg = "Authenticating via " + m.cfg.Auth.Browser + "..."
	browser := m.cfg.Auth.Browser
	return func() tea.Msg {
		jar, err := auth.ExtractCookies(context.Background(), browser)
		if err != nil {
			return musicAuthFailedMsg{err: err}
		}
		httpClient := auth.HTTPClient(jar)
		newClient, err := youtube.NewMusicClient(httpClient)
		if err != nil {
			return musicAuthFailedMsg{err: err}
		}
		return musicAuthSuccessMsg{client: newClient}
	}
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
