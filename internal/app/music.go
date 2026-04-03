package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/auth"
	"github.com/deathmaz/ytui/internal/config"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/ui/detail"
	"github.com/deathmaz/ytui/internal/ui/search"
	"github.com/deathmaz/ytui/internal/ui/urlinput"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

type musicKeyMap struct{}

func (k musicKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "play")),
		key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "play album")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next section")),
		key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "load all")),
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "auth")),
		key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "open URL")),
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
	musicTabSong
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
	albumPage     *youtube.MusicAlbumPage
	albumList     list.Model
	thumbTransmit string
	thumbPlace    string
	thumbPending  bool
	// Song detail (comments)
	songDetail detail.Model
	loaded     bool
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

const (
	albumThumbCols = 13
	albumThumbRows = 7
)

type musicClearTransmitMsg struct{}

func musicClearTransmitCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return musicClearTransmitMsg{}
	})
}

func bestMusicThumbnail(thumbs []youtube.Thumbnail) string {
	if len(thumbs) == 0 {
		return ""
	}
	best := thumbs[0]
	for _, t := range thumbs[1:] {
		if t.Width > best.Width {
			best = t
		}
	}
	return best.URL
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
	onFixedView  bool // true = fixed view (home/library/search), false = dynamic tab
	width        int
	height       int
	keys         KeyMap
	help         help.Model
	client       *youtube.MusicClient
	ytClient     youtube.Client
	cfg          *config.Config
	imgR         *ytimage.Renderer
	urlInput     urlinput.Model

	// Fixed views
	activeFixed musicFixedView
	search      search.Model
	spinner     spinner.Model

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
	pendingOpen    *youtube.ParsedURL
	statusMsg      string
	statusSeq      int
}

// NewMusic creates a new root model for music mode.
func NewMusic(client *youtube.MusicClient, ytClient youtube.Client, cfg *config.Config, imgR *ytimage.Renderer, opts Options) *MusicModel {
	h := help.New()
	h.ShortSeparator = "  "

	s := search.New(newMusicSearchConfig(client))
	if opts.SearchQuery != "" {
		s.SetQuery(opts.SearchQuery)
	}
	m := &MusicModel{
		onFixedView: true,
		activeFixed: musicViewSearch,
		keys:        DefaultKeyMap(),
		help:        h,
		client:      client,
		ytClient:    ytClient,
		cfg:         cfg,
		imgR:        imgR,
		search:      s,
		urlInput:    urlinput.New(),
		spinner:     styles.NewSpinner(),
		pendingOpen: opts.OpenURL,
	}

	return m
}

func (m *MusicModel) Init() tea.Cmd {
	cmd := initCmds(
		m.cfg.Auth.AuthOnStartup,
		&m.pendingOpen,
		m.search.Init(),
		m.authenticate,
		m.openParsedURL,
		m.search.Query(),
		m.search.Refresh,
	)
	switch m.activeFixed {
	case musicViewHome:
		return tea.Batch(cmd, m.loadHome())
	case musicViewLibrary:
		return tea.Batch(cmd, m.loadLibrary())
	}
	return cmd
}

func (m *MusicModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
		m.height = wsm.Height
		m.help.Width = wsm.Width
		m.resizeViews()
	}

	if cmd, handled := handleURLInput(msg, &m.urlInput); handled {
		return m, cmd
	}

	if searchCmd, ok := handleSearchFocused(msg, &m.search, m.onFixedView && m.activeFixed == musicViewSearch, m.keys); ok {
		return m, searchCmd
	}

	// Delegate to song detail tab
	if tab := m.activeTab(); tab != nil && tab.kind == musicTabSong {
		handled := true
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch {
			case key.Matches(keyMsg, m.keys.ForceQuit):
				return m, tea.Quit
			case key.Matches(keyMsg, m.keys.Quit):
				return m, tea.Quit
			case key.Matches(keyMsg, m.keys.Back):
				m.closeActiveTab()
				return m, nil
			case key.Matches(keyMsg, m.keys.Play):
				return m, playVideoCmd(youtube.VideoURL(tab.browseID), "", m.cfg.Player.EffectiveCommand(true), m.cfg.Player.EffectiveArgs(true))
			default:
				// Let tab number keys fall through to main handler
				if k := keyMsg.String(); len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
					handled = false
				}
			}
		}
		if handled {
			var cmd tea.Cmd
			tab.songDetail, cmd = tab.songDetail.Update(msg)
			cmds = append(cmds, cmd)
			if vlm, ok := msg.(detail.VideoLoadedMsg); ok && vlm.Err == nil && vlm.Video != nil && tab.title == "" {
				tab.title = vlm.Video.Title
			}
			return m, tea.Batch(cmds...)
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, m.keys.ForceQuit) {
			return m, tea.Quit
		}

		// Global keys
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		case key.Matches(msg, m.keys.Back):
			if !m.onFixedView {
				m.closeActiveTab()
				return m, nil
			}
		case key.Matches(msg, m.keys.Play):
			return m, m.playSelected()
		case msg.String() == "P":
			return m, m.playAlbum()
		case key.Matches(msg, m.keys.Auth):
			return m, m.authenticate()
		case key.Matches(msg, m.keys.OpenURL):
			return m, m.urlInput.Show(m.width, m.height)
		case key.Matches(msg, m.keys.Search), msg.String() == "/":
			m.onFixedView = true
			m.activeFixed = musicViewSearch
			m.search.Focus()
			return m, m.search.Init()
		case msg.String() == "enter":
			return m, m.openSelected()
		}

		// Tab keys: 1=Home, 2=Library, 3=Search, 4+=dynamic tabs
		if k := msg.String(); len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
			idx := int(k[0] - '1')
			if idx <= 2 {
				m.onFixedView = true
				m.activeFixed = musicFixedView(idx)
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

		// Delegate key events to active list
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

	case musicItemSelectedMsg:
		return m, m.openMusicItem(msg.item)

	case urlinput.SubmitMsg:
		return m, m.openParsedURL(&msg.Parsed)

	case urlinput.CancelMsg:

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
				if msg.Artist.Name != "" {
					tab.title = msg.Artist.Name
				}
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
		var fetchCmd tea.Cmd
		for i := range m.tabs {
			tab := &m.tabs[i]
			if tab.browseID == msg.BrowseID && tab.kind == musicTabAlbum && !tab.loaded {
				tab.albumPage = msg.Album
				tab.albumList = m.buildAlbumList(msg.Album)
				if msg.Album.Title != "" {
					tab.title = msg.Album.Title
				}
				tab.loaded = true
				m.resizeViews()
				if m.imgR != nil && len(msg.Album.Thumbnails) > 0 {
					thumbURL := bestMusicThumbnail(msg.Album.Thumbnails)
					if thumbURL != "" {
						if tx, pl := m.imgR.Get(thumbURL); pl != "" {
							tab.thumbTransmit = tx
							tab.thumbPlace = pl
							fetchCmd = musicClearTransmitCmd()
						} else {
							tab.thumbPending = true
							fetchCmd = m.imgR.FetchCmd(thumbURL, albumThumbCols, albumThumbRows)
						}
					}
				}
				break
			}
		}
		return m, fetchCmd

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
		return m, playVideoCmd(msg.url, "", m.cfg.Player.EffectiveCommand(true), m.cfg.Player.EffectiveArgs(true))

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
		homeCmd := m.loadHome()
		libCmd := m.loadLibrary()
		var openCmd tea.Cmd
		if m.pendingOpen != nil {
			openCmd = m.openParsedURL(m.pendingOpen)
			m.pendingOpen = nil
		}
		return m, tea.Batch(m.setStatus("Authenticated via "+m.cfg.Auth.Browser, 3*time.Second), homeCmd, libCmd, openCmd)

	case musicAuthFailedMsg:
		m.authenticating = false
		return m, m.setStatus("Auth failed: "+msg.err.Error(), 5*time.Second)

	case ytimage.ThumbnailLoadedMsg:
		for i := range m.tabs {
			tab := &m.tabs[i]
			if tab.kind == musicTabAlbum && tab.loaded && tab.thumbPending {
				if len(tab.albumPage.Thumbnails) > 0 && bestMusicThumbnail(tab.albumPage.Thumbnails) == msg.URL {
					tab.thumbPending = false
					if msg.Err == nil && msg.Placeholder != "" {
						m.imgR.Store(msg.URL, msg.TransmitStr, msg.Placeholder)
						tab.thumbTransmit = msg.TransmitStr
						tab.thumbPlace = msg.Placeholder
						cmds = append(cmds, musicClearTransmitCmd())
					}
					break
				}
			}
		}

	case musicClearTransmitMsg:
		for i := range m.tabs {
			m.tabs[i].thumbTransmit = ""
		}

	case clearStatusMsg:
		if handleClearStatus(msg, m.statusSeq, &m.statusMsg) {
			m.resizeViews()
		}

	case spinner.TickMsg:
		if m.homeLoading || m.libraryLoading || m.pageLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Always delegate to search model so it receives all messages (spinner ticks, results, etc.)
	if m.onFixedView && m.activeFixed == musicViewSearch {
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *MusicModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	if m.urlInput.IsActive() {
		return m.urlInput.View()
	}

	var statusLine string
	if m.statusMsg != "" {
		statusLine = styles.Accent.Render(m.statusMsg)
	}

	view := composeSections(
		m.renderTabs(),
		m.renderContent(),
		statusLine,
		statusBarStyle.Render(m.help.View(musicKeyMap{})),
	)

	if tab := m.activeTab(); tab != nil && tab.thumbTransmit != "" {
		view = tab.thumbTransmit + view
	}

	return view
}

func (m *MusicModel) loadHome() tea.Cmd {
	if m.homeLoaded || m.homeLoading {
		return nil
	}
	m.homeLoading = true
	client := m.client
	return tea.Batch(m.spinner.Tick, func() (msg tea.Msg) {
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
	cmds = append(cmds, m.spinner.Tick)
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
			return m.openTab(musicTabSong, it.Title, it.VideoID)
		}
	case youtube.MusicPlaylist:
		if it.BrowseID != "" {
			return m.openTab(musicTabAlbum, it.Title, it.BrowseID)
		}
	}
	return nil
}

func (m *MusicModel) openParsedURL(p *youtube.ParsedURL) tea.Cmd {
	if p == nil {
		return nil
	}
	switch p.Kind {
	case youtube.URLVideo:
		return m.openTab(musicTabSong, "", p.ID)
	case youtube.URLPlaylist:
		browseID := p.ID
		if !strings.HasPrefix(browseID, "VL") {
			browseID = "VL" + browseID
		}
		return m.openTab(musicTabAlbum, "", browseID)
	case youtube.URLChannel:
		return m.openTab(musicTabArtist, "", p.ID)
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

	tab := musicTab{
		kind:     kind,
		title:    title,
		browseID: browseID,
	}

	// Song tabs use the detail.Model directly
	if kind == musicTabSong {
		d := detail.New(m.ytClient, m.imgR)
		d.SetSize(m.width, m.contentHeight())
		tab.songDetail = d
		tab.loaded = true
		m.tabs = append(m.tabs, tab)
		m.activeTabIdx = len(m.tabs) - 1
		m.onFixedView = false
		return m.tabs[m.activeTabIdx].songDetail.LoadVideo(browseID)
	}

	m.tabs = append(m.tabs, tab)
	m.activeTabIdx = len(m.tabs) - 1
	m.onFixedView = false
	m.pageLoading = true

	client := m.client
	switch kind {
	case musicTabArtist:
		return tea.Batch(m.spinner.Tick, func() tea.Msg {
			artist, err := client.GetArtist(context.Background(), browseID)
			return musicArtistLoadedMsg{BrowseID: browseID, Artist: artist, Err: err}
		})
	case musicTabAlbum:
		return tea.Batch(m.spinner.Tick, func() tea.Msg {
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
		switch tab.kind {
		case musicTabAlbum:
			icon = "◉"
		case musicTabSong:
			icon = "♪"
		}
		title := tab.title
		if title == "" {
			title = "..."
		}
		label := fmt.Sprintf("[%d] %s", i+4, shared.Truncate(icon+" "+title, 22))
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
		return m.spinner.View() + " Loading..."
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
			return m.spinner.View() + " Loading..."
		}
		switch tab.kind {
		case musicTabArtist:
			return m.renderArtistPage(tab)
		case musicTabAlbum:
			return m.renderAlbumPage(tab)
		case musicTabSong:
			return tab.songDetail.View()
		}
	}
	return ""
}

func (m *MusicModel) renderHome() string {
	if m.homeLoading {
		return m.spinner.View() + " Loading home..."
	}
	if !m.homeLoaded || len(m.homeSubs) == 0 {
		return styles.Dim.Render("Home feed not loaded")
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
		return m.spinner.View() + " Loading library..."
	}
	if !m.libraryLoaded || len(m.librarySubs) == 0 {
		return styles.Dim.Render("Press 'a' to authenticate to view library")
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
	return m.search.View()
}

var (
	subTabStyle       = styles.SubTab
	activeSubTabStyle = styles.ActiveSubTab
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
	album := tab.albumPage
	var infoLines []string

	// Album art + title/meta side by side
	title := styles.Title.Render(album.Title)
	meta := ""
	if album.Artist != "" {
		meta = styles.Subtitle.Render(album.Artist)
	}
	if album.Year != "" {
		if meta != "" {
			meta += "  "
		}
		meta += styles.Dim.Render(album.Year)
	}
	var infoParts []string
	if album.AlbumType != "" {
		infoParts = append(infoParts, album.AlbumType)
	}
	if album.TrackCount != "" {
		infoParts = append(infoParts, album.TrackCount)
	}
	if album.Duration != "" {
		infoParts = append(infoParts, album.Duration)
	}

	textBlock := "\n" + title
	if meta != "" {
		textBlock += "\n" + meta
	}
	if len(infoParts) > 0 {
		textBlock += "\n" + styles.Dim.Render(strings.Join(infoParts, " • "))
	}
	if album.Description != "" {
		wrapped := lipgloss.NewStyle().Width(m.width - albumThumbCols - 4).Render(album.Description)
		textBlock += "\n" + styles.Dim.Render(wrapped)
	}

	if tab.thumbPlace != "" {
		infoLines = append(infoLines, lipgloss.JoinHorizontal(lipgloss.Top, tab.thumbPlace+"  ", textBlock))
	} else if tab.thumbPending {
		// Reserve space for the image while it loads
		placeholder := lipgloss.NewStyle().Width(albumThumbCols).Height(albumThumbRows).Render("")
		infoLines = append(infoLines, lipgloss.JoinHorizontal(lipgloss.Top, placeholder+"  ", textBlock))
	} else {
		infoLines = append(infoLines, textBlock)
	}

	header := lipgloss.JoinVertical(lipgloss.Left, infoLines...)

	ch := m.contentHeight()
	tab.albumList.SetSize(m.width, ch-lipgloss.Height(header))
	return lipgloss.JoinVertical(lipgloss.Left, header, tab.albumList.View())
}

func (m *MusicModel) contentHeight() int {
	return calcContentHeight(m.height, m.renderTabs(),
		statusBarStyle.Render(m.help.View(musicKeyMap{})),
		m.statusMsg != "")
}

func (m *MusicModel) resizeViews() {
	if m.width == 0 {
		return
	}
	ch := m.contentHeight()
	m.search.SetSize(m.width, ch)
	// Resize song detail tabs
	for i := range m.tabs {
		if m.tabs[i].kind == musicTabSong {
			m.tabs[i].songDetail.SetSize(m.width, ch)
		}
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
			sel = m.search.SelectedItem()
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
	return m.playItem(*ptr)
}

func (m *MusicModel) playAlbum() tea.Cmd {
	tab := m.activeTab()
	if tab == nil || tab.kind != musicTabAlbum || !tab.loaded || tab.albumPage == nil {
		return m.setStatus("No album to play", 2*time.Second)
	}
	if tab.albumPage.PlaylistID != "" {
		return playVideoCmd(youtube.MusicPlaylistURL(tab.albumPage.PlaylistID), "", m.cfg.Player.EffectiveCommand(true), m.cfg.Player.EffectiveArgs(true))
	}
	if len(tab.albumPage.Tracks) > 0 && tab.albumPage.Tracks[0].VideoID != "" {
		return playVideoCmd(youtube.VideoURL(tab.albumPage.Tracks[0].VideoID), "", m.cfg.Player.EffectiveCommand(true), m.cfg.Player.EffectiveArgs(true))
	}
	return m.setStatus("No playable tracks", 2*time.Second)
}

// playItem is the single play entry point for all music items.
func (m *MusicModel) playItem(it youtube.MusicItem) tea.Cmd {
	// Albums/playlists: browse to get the full playlist URL
	if it.BrowseID != "" && (it.Type == youtube.MusicAlbum || it.Type == youtube.MusicPlaylist) {
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
					return musicPlayReadyMsg{url: youtube.MusicPlaylistURL(playlistID)}
				}
				if len(tracks) > 0 && tracks[0].VideoID != "" {
					return musicPlayReadyMsg{url: youtube.VideoURL(tracks[0].VideoID)}
				}
				return musicPlayReadyMsg{err: fmt.Errorf("no playable tracks found")}
			},
		)
	}

	// Songs/videos: play directly
	if it.VideoID != "" {
		return playVideoCmd(youtube.VideoURL(it.VideoID), "", m.cfg.Player.EffectiveCommand(true), m.cfg.Player.EffectiveArgs(true))
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

type musicItemSelectedMsg struct {
	item youtube.MusicItem
}

func newMusicSearchConfig(client *youtube.MusicClient) search.Config {
	return search.Config{
		Placeholder: "Search YouTube Music...",
		Delegate:    musicDelegate{},
		SearchFn: func(ctx context.Context, query, pageToken string) search.SearchResult {
			result, err := client.Search(ctx, query, pageToken)
			if err != nil {
				return search.SearchResult{Err: err}
			}
			var items []list.Item
			if result.TopResult != nil {
				items = append(items, musicItem{item: *result.TopResult})
			}
			for _, shelf := range result.Shelves {
				for _, it := range shelf.Items {
					items = append(items, musicItem{item: it})
				}
			}
			return search.SearchResult{
				Items:     items,
				NextToken: result.NextToken,
			}
		},
		SelectFn: func(item list.Item) tea.Msg {
			if mi, ok := item.(musicItem); ok {
				return musicItemSelectedMsg{item: mi.item}
			}
			return nil
		},
	}
}

func (m *MusicModel) setStatus(msg string, clearAfter time.Duration) tea.Cmd {
	cmd := setStatusCmd(&m.statusSeq, &m.statusMsg, msg, clearAfter)
	m.resizeViews()
	return cmd
}
