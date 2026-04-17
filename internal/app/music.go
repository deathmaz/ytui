package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/config"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/state"
	"github.com/deathmaz/ytui/internal/ui/detail"
	"github.com/deathmaz/ytui/internal/ui/picker"
	"github.com/deathmaz/ytui/internal/ui/search"
	"github.com/deathmaz/ytui/internal/ui/urlinput"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)


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
	// isAbout marks the synthetic About entry on artist pages. Rendered by
	// artistAboutView instead of the list; load-more and list navigation skip
	// it. Prefer this over string-matching the title in case an upstream
	// shelf is ever named "About".
	isAbout bool
}

type musicTab struct {
	kind         musicTabKind
	title        string
	browseID     string
	needsLoad    bool // true for restored tabs that haven't loaded yet
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
	client       youtube.MusicAPI
	ytClient     youtube.Client
	cfg          *config.Config
	imgR         *ytimage.Renderer
	thumbList    *shared.ThumbList
	urlInput     urlinput.Model

	// Fixed views
	activeFixed musicFixedView
	search      search.Model
	spinner     spinner.Model
	picker      picker.Model

	pendingSubscribe *subscribeTarget

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
	tabs        TabSet[musicTab]
	pageLoading bool

	authenticating  bool
	pendingState
	startupWarning  string
	status          StatusManager
}

// NewMusic creates a new root model for music mode.
func NewMusic(client youtube.MusicAPI, ytClient youtube.Client, cfg *config.Config, imgR *ytimage.Renderer, opts Options) *MusicModel {
	h := help.New()
	h.ShortSeparator = "  "

	thumbH := cfg.Thumbnails.Height
	if thumbH <= 0 {
		thumbH = 5
	}

	var thumbList *shared.ThumbList
	if cfg.Thumbnails.Enabled {
		thumbList = shared.NewThumbList(ytimage.NewRenderer(), musicThumbURL, thumbH)
	}

	s := search.New(newMusicSearchConfig(client, thumbList, thumbH))
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
		thumbList:   thumbList,
		search:      s,
		urlInput:    urlinput.New(),
		picker:      picker.New(),
		spinner:     styles.NewSpinner(),
		tabs:           NewTabSet[musicTab](maxMusicTabs, func(t *musicTab) string { return t.browseID }),
		pendingState: pendingState{
			pendingOpen:    opts.OpenURL,
			pendingRestore: loadSavedTabs(cfg, "music"),
		},
		startupWarning: opts.Warning,
	}

	return m
}

func (m *MusicModel) Init() tea.Cmd {
	cmd := initCmds(
		m.cfg.Auth.AuthOnStartup,
		m.hasPending(),
		m.drainPending,
		m.search.Init(),
		m.authenticate,
		m.search.Query(),
		m.search.Refresh,
	)
	if m.startupWarning != "" {
		cmd = tea.Batch(cmd, m.setStatus(m.startupWarning, 10*time.Second))
		m.startupWarning = ""
	}
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
		HandleWindowSize(wsm, &m.width, &m.height, &m.help)
		m.resizeViews()
	}

	if cmd, handled := handlePickerKey(msg, &m.picker); handled {
		return m, cmd
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
		// Let subscribe-related messages fall through to the main switch;
		// otherwise forwarding them into detail.Update swallows them and the
		// picker selection never drives runSubscription.
		switch msg.(type) {
		case picker.SelectedMsg, picker.CancelledMsg, subscribeResultMsg:
			handled = false
		}
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if action, ok := HandleGlobalKey(keyMsg, m.keys); ok {
				switch action {
				case KeyQuit:
					return m, tea.Quit
				default:
					// Let other global keys fall through
					handled = false
				}
			} else {
				switch {
				case key.Matches(keyMsg, m.keys.Back):
					m.closeActiveTab()
					return m, m.loadRestoredTab()
				case key.Matches(keyMsg, m.keys.Play):
					return m, playVideoCmd(youtube.VideoURL(tab.browseID), "", m.cfg.Player.EffectiveCommand(true), m.cfg.Player.EffectiveArgs(true))
				case key.Matches(keyMsg, m.keys.Subscribe):
					// Duplicated in the outer switch below; the song-tab delegate
					// block returns early for keypresses, so the outer S binding
					// is unreachable once a song tab is active.
					return m, m.openSubscribePicker()
				default:
					// Let tab number keys fall through to main handler
					if k := keyMsg.String(); len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
						handled = false
					}
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
		// Shared global keys
		if action, ok := HandleGlobalKey(msg, m.keys); ok {
			switch action {
			case KeyQuit:
				return m, tea.Quit
			case KeyHelpToggle:
				m.help.ShowAll = !m.help.ShowAll
				return m, nil
			case KeyAuth:
				return m, m.authenticate()
			case KeyOpenURL:
				return m, m.urlInput.Show(m.width, m.height)
			case KeySearch:
				m.onFixedView = true
				m.activeFixed = musicViewSearch
				m.search.Focus()
				return m, tea.Batch(m.search.Init(), m.refetchVisibleThumbs())
			}
		}

		// Music-specific keys
		switch {
		case key.Matches(msg, m.keys.Back):
			if !m.onFixedView {
				m.closeActiveTab()
				return m, tea.Batch(m.loadRestoredTab(), m.refetchVisibleThumbs())
			}
		case key.Matches(msg, m.keys.Play):
			return m, m.playSelected()
		case key.Matches(msg, m.keys.PlayAlbum):
			return m, m.playAlbum()
		case key.Matches(msg, m.keys.Enter):
			return m, m.openSelected()
		case key.Matches(msg, m.keys.Subscribe):
			return m, m.openSubscribePicker()
		}

		// Tab keys: 1=Home, 2=Library, 3=Search, 4+=dynamic tabs
		if k := msg.String(); len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
			idx := int(k[0] - '1')
			if idx <= 2 {
				m.onFixedView = true
				m.activeFixed = musicFixedView(idx)
				switch m.activeFixed {
				case musicViewHome:
					return m, tea.Batch(m.loadHome(), m.refetchVisibleThumbs())
				case musicViewLibrary:
					return m, tea.Batch(m.loadLibrary(), m.refetchVisibleThumbs())
				}
				return m, m.refetchVisibleThumbs()
			}
			tabIdx := idx - 3
			if tabIdx < m.tabs.Len() {
				m.onFixedView = false
				m.tabs.SetActive(tabIdx)
				return m, tea.Batch(m.loadRestoredTab(), m.refetchVisibleThumbs())
			}
		}

		// Sub-tab navigation for artist pages
		if !m.onFixedView {
			if tab := m.activeTab(); tab != nil && tab.kind == musicTabArtist && tab.loaded {
				if key.Matches(msg, m.keys.NextTab) {
					if tab.activeSubTab < len(tab.artistSubs)-1 {
						tab.activeSubTab++
						m.resizeViews()
					}
					return m, m.refetchVisibleThumbs()
				}
				if key.Matches(msg, m.keys.PrevTab) {
					if tab.activeSubTab > 0 {
						tab.activeSubTab--
						m.resizeViews()
					}
					return m, m.refetchVisibleThumbs()
				}
				if key.Matches(msg, m.keys.LoadMore) {
					return m, m.loadMoreForSubTab(tab)
				}
			}
		}

		// Sub-tab navigation for home and library views
		if m.onFixedView && m.activeFixed == musicViewHome && m.homeLoaded {
			if key.Matches(msg, m.keys.NextTab) {
				if m.homeSubIdx < len(m.homeSubs)-1 {
					m.homeSubIdx++
				}
				return m, m.refetchVisibleThumbs()
			}
			if key.Matches(msg, m.keys.PrevTab) {
				if m.homeSubIdx > 0 {
					m.homeSubIdx--
				}
				return m, m.refetchVisibleThumbs()
			}
		}
		if m.onFixedView && m.activeFixed == musicViewLibrary && m.libraryLoaded {
			if key.Matches(msg, m.keys.NextTab) {
				if m.librarySubIdx < len(m.librarySubs)-1 {
					m.librarySubIdx++
				}
				return m, m.refetchVisibleThumbs()
			}
			if key.Matches(msg, m.keys.PrevTab) {
				if m.librarySubIdx > 0 {
					m.librarySubIdx--
				}
				return m, m.refetchVisibleThumbs()
			}
			if key.Matches(msg, m.keys.LoadMore) {
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
					if shared.ShouldLoadMore(m.librarySubs[m.librarySubIdx].list, 5) {
						cmds = append(cmds, m.loadMoreLibrary())
					}
				}
			}
		} else if tab := m.activeTab(); tab != nil {
			switch tab.kind {
			case musicTabArtist:
				if tab.loaded && tab.activeSubTab < len(tab.artistSubs) {
					var cmd tea.Cmd
					tab.artistSubs[tab.activeSubTab].list, cmd = tab.artistSubs[tab.activeSubTab].list.Update(msg)
					cmds = append(cmds, cmd)
					if shared.ShouldLoadMore(tab.artistSubs[tab.activeSubTab].list, 5) {
						cmds = append(cmds, m.loadMoreForSubTab(tab))
					}
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
		m.homeSubs = shelvesToSubTabs(msg.Shelves, m.musicListDelegate())
		m.homeSubIdx = 0
		for i := range m.homeSubs {
			cmds = append(cmds, m.thumbList.TriggerFetch(&m.homeSubs[i].list, msg))
		}
		return m, tea.Batch(cmds...)

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
		m.status.Clear()
		if msg.Index < len(m.librarySubs) {
			cmds = append(cmds, m.thumbList.TriggerFetch(&m.librarySubs[msg.Index].list, msg))
		}
		return m, tea.Batch(cmds...)

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
		for i := range m.tabs.All() {
			tab := m.tabs.At(i)
			if tab.browseID == msg.BrowseID && tab.kind == musicTabArtist && !tab.loaded {
				tab.artistPage = msg.Artist
				tab.artistSubs = m.buildArtistSubTabs(msg.Artist)
				if msg.Artist.Name != "" {
					tab.title = msg.Artist.Name
				}
				tab.loaded = true
				m.resizeViews()
				for j := range tab.artistSubs {
					cmds = append(cmds, m.thumbList.TriggerFetch(&tab.artistSubs[j].list, msg))
				}
				break
			}
		}
		return m, tea.Batch(cmds...)

	case musicAlbumLoadedMsg:
		m.pageLoading = false
		if msg.Err != nil {
			return m, m.setStatus("Error: "+msg.Err.Error(), 5*time.Second)
		}
		var fetchCmd tea.Cmd
		for i := range m.tabs.All() {
			tab := m.tabs.At(i)
			if tab.browseID == msg.BrowseID && tab.kind == musicTabAlbum && !tab.loaded {
				tab.albumPage = msg.Album
				tab.albumList = m.buildAlbumList(msg.Album)
				if msg.Album.Title != "" {
					tab.title = msg.Album.Title
				}
				tab.loaded = true
				m.resizeViews()
				if m.imgR != nil && len(msg.Album.Thumbnails) > 0 {
					thumbURL := shared.BestThumbnailURL(msg.Album.Thumbnails)
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
		m.status.Clear()
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

	case picker.SelectedMsg:
		switch msg.Target {
		case picker.TargetSubscribe:
			if t := m.pendingSubscribe; t != nil {
				m.pendingSubscribe = nil
				return m, m.runSubscription(t.channelID, t.name, msg.Key == subKeySubscribe)
			}
		default:
			return m, m.setStatus(fmt.Sprintf("unknown picker target: %d", msg.Target), 3*time.Second)
		}
		return m, nil

	case picker.CancelledMsg:
		if msg.Target == picker.TargetSubscribe {
			m.pendingSubscribe = nil
		}
		return m, nil

	case subscribeResultMsg:
		return m, m.handleSubscribeResult(msg)

	case musicPlayReadyMsg:
		if msg.err != nil {
			return m, m.setStatus("Play error: "+msg.err.Error(), 5*time.Second)
		}
		return m, playVideoCmd(msg.url, "", m.cfg.Player.EffectiveCommand(true), m.cfg.Player.EffectiveArgs(true))

	case AuthResult:
		return m, HandleAuthResult(msg, &m.authenticating, &m.status, m.cfg.Auth.Browser,
			m.drainPending,
			func(httpClient *http.Client) error {
				newClient, err := youtube.NewMusicClient(httpClient)
				if err != nil {
					return err
				}
				m.client = newClient
				if newYtClient, err := youtube.NewInnerTubeClient(httpClient); err == nil {
					m.ytClient = newYtClient
				}
				// Reset home/library so they reload when focused
				m.homeLoaded = false
				m.homeLoading = false
				m.libraryLoaded = false
				m.libraryLoading = false
				m.librarySubs = nil
				m.librarySubIdx = 0
				return nil
			},
			func() tea.Cmd {
				if m.onFixedView {
					switch m.activeFixed {
					case musicViewHome:
						return m.loadHome()
					case musicViewLibrary:
						return m.loadLibrary()
					}
				}
				return nil
			},
			m.resizeViews,
		)

	case ytimage.ThumbnailLoadedMsg:
		// List thumbnails (search, home, library, artist shelves)
		m.thumbList.HandleMsg(msg)
		// Album detail page thumbnails
		if m.imgR != nil && m.imgR.HandleLoaded(msg) {
			for i := range m.tabs.All() {
				tab := m.tabs.At(i)
				if tab.kind == musicTabAlbum && tab.loaded && tab.thumbPending {
					if len(tab.albumPage.Thumbnails) > 0 && shared.BestThumbnailURL(tab.albumPage.Thumbnails) == msg.URL {
						tab.thumbPending = false
						tx, pl := m.imgR.Get(msg.URL)
						tab.thumbTransmit = tx
						tab.thumbPlace = pl
						cmds = append(cmds, musicClearTransmitCmd())
						break
					}
				}
			}
		}

	case musicClearTransmitMsg:
		for i := range m.tabs.All() {
			m.tabs.At(i).thumbTransmit = ""
		}

	case clearStatusMsg:
		if m.status.HandleClear(msg) {
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
	var statusLine string
	if m.status.Msg != "" {
		statusLine = styles.Accent.Render(m.status.Msg)
	}

	view := RenderShell(
		m.width,
		[]ModalView{&m.urlInput, &m.picker},
		m.renderTabs,
		m.renderContent,
		statusLine,
		statusBarStyle.Render(m.help.View(musicHelpAdapter{m.keys})),
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
	m.thumbList.Invalidate()
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
		return nil
	}
	m.libraryLoading = true
	m.thumbList.Invalidate()

	// Initialize sub-tabs with empty lists
	sections := youtube.LibrarySections
	m.librarySubs = make([]subTab, len(sections))
	for i, sec := range sections {
		m.librarySubs[i] = subTab{title: sec.Title, list: shared.NewList(m.musicListDelegate())}
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

func shelvesToSubTabs(shelves []youtube.MusicShelf, delegate list.ItemDelegate) []subTab {
	var subs []subTab
	for _, shelf := range shelves {
		if len(shelf.Items) == 0 {
			continue
		}
		l := shared.NewList(delegate)
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

// refetchVisibleThumbs returns a cmd that re-fetches thumbnails for visible
// items whose cache entries were evicted by the LRU.
func (m *MusicModel) refetchVisibleThumbs() tea.Cmd {
	if m.thumbList == nil {
		return nil
	}
	if m.onFixedView {
		switch m.activeFixed {
		case musicViewHome:
			if m.homeLoaded && m.homeSubIdx < len(m.homeSubs) {
				return m.thumbList.RefetchCmd(m.homeSubs[m.homeSubIdx].list)
			}
		case musicViewLibrary:
			if m.libraryLoaded && m.librarySubIdx < len(m.librarySubs) {
				return m.thumbList.RefetchCmd(m.librarySubs[m.librarySubIdx].list)
			}
		case musicViewSearch:
			return m.search.RefetchThumbs()
		}
		return nil
	}
	if tab := m.tabs.Active(); tab != nil && tab.kind == musicTabArtist && tab.loaded {
		if tab.activeSubTab < len(tab.artistSubs) {
			return m.thumbList.RefetchCmd(tab.artistSubs[tab.activeSubTab].list)
		}
	}
	return nil
}

func (m *MusicModel) activeTab() *musicTab {
	if m.onFixedView {
		return nil
	}
	return m.tabs.Active()
}

// drainPending wraps pendingState.drain with this mode's open/restore fns.
func (m *MusicModel) drainPending() []tea.Cmd {
	return m.drain(m.openParsedURL, m.restoreTabs)
}

func (m *MusicModel) closeActiveTab() {
	if m.onFixedView || m.tabs.Len() == 0 {
		return
	}
	_, empty := m.tabs.Close(m.tabs.ActiveIdx())
	if empty {
		m.onFixedView = true
	}
	saveTabState(m.cfg, "music", m.tabEntries())
}

// tabEntries returns the current dynamic tabs as persistable entries.
func (m *MusicModel) tabEntries() []state.TabEntry {
	var entries []state.TabEntry
	for _, tab := range m.tabs.All() {
		var kind string
		switch tab.kind {
		case musicTabArtist:
			kind = state.KindArtist
		case musicTabAlbum:
			kind = state.KindAlbum
		case musicTabSong:
			kind = state.KindSong
		default:
			continue
		}
		entries = append(entries, state.TabEntry{Kind: kind, ID: tab.browseID, Title: tab.title})
	}
	return entries
}

// restoreTabs creates tab entries from a previous session without loading
// their content. Loading is deferred until the user switches to the tab
// (via loadRestoredTab), because messages from background loads would be
// dropped while the user is on a different view.
func (m *MusicModel) restoreTabs(entries []state.TabEntry) tea.Cmd {
	for _, e := range entries {
		var kind musicTabKind
		switch e.Kind {
		case state.KindArtist:
			kind = musicTabArtist
		case state.KindAlbum:
			kind = musicTabAlbum
		case state.KindSong:
			kind = musicTabSong
		default:
			continue
		}
		tab := musicTab{
			kind:      kind,
			title:     e.Title,
			browseID:  e.ID,
			needsLoad: true,
		}
		if kind == musicTabSong {
			tab.songDetail = detail.New(m.ytClient, m.imgR)
		}
		if _, err := m.tabs.Open(tab); err != nil {
			break
		}
	}
	return nil
}

// loadRestoredTab triggers the initial load for the active tab if it was
// restored from a previous session and hasn't loaded yet.
func (m *MusicModel) loadRestoredTab() tea.Cmd {
	tab := m.tabs.Active()
	if tab == nil || !tab.needsLoad {
		return nil
	}
	tab.needsLoad = false
	switch tab.kind {
	case musicTabSong:
		tab.songDetail.SetSize(m.width, m.contentHeight())
		tab.loaded = true
		return tab.songDetail.LoadVideo(tab.browseID)
	case musicTabArtist:
		m.pageLoading = true
		m.thumbList.Invalidate()
		client := m.client
		return tea.Batch(m.spinner.Tick, func() tea.Msg {
			artist, err := client.GetArtist(context.Background(), tab.browseID)
			return musicArtistLoadedMsg{BrowseID: tab.browseID, Artist: artist, Err: err}
		})
	case musicTabAlbum:
		m.pageLoading = true
		m.thumbList.Invalidate()
		client := m.client
		return tea.Batch(m.spinner.Tick, func() tea.Msg {
			album, err := client.GetAlbum(context.Background(), tab.browseID)
			return musicAlbumLoadedMsg{BrowseID: tab.browseID, Album: album, Err: err}
		})
	}
	return nil
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
	if idx, found := m.tabs.Find(browseID); found {
		m.onFixedView = false
		m.tabs.SetActive(idx)
		m.thumbList.Invalidate()
		return m.loadRestoredTab()
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
		idx, err := m.tabs.Open(tab)
		if err != nil {
			return m.setStatus("Max tabs reached (close one with Esc)", 3*time.Second)
		}
		m.tabs.SetActive(idx)
		m.onFixedView = false
		m.thumbList.Invalidate()
		saveTabState(m.cfg, "music", m.tabEntries())
		return m.tabs.Active().songDetail.LoadVideo(browseID)
	}

	idx, err := m.tabs.Open(tab)
	if err != nil {
		return m.setStatus("Max tabs reached (close one with Esc)", 3*time.Second)
	}
	m.tabs.SetActive(idx)
	m.onFixedView = false
	m.pageLoading = true
	m.thumbList.Invalidate()
	saveTabState(m.cfg, "music", m.tabEntries())

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

const artistAboutTabTitle = "About"

func (m *MusicModel) buildArtistSubTabs(artist *youtube.MusicArtistPage) []subTab {
	shelves := shelvesToSubTabs(artist.Shelves, m.musicListDelegate())
	// Prepend About as the landing sub-tab so the artist's name, subscriber
	// count, subscription state, and description are visible without extra
	// navigation. The About entry has no list; renderArtistPage special-cases
	// it.
	about := []subTab{{title: artistAboutTabTitle, list: shared.NewList(m.musicListDelegate()), isAbout: true}}
	return append(about, shelves...)
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
	for i, tab := range m.tabs.All() {
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
		if !m.onFixedView && m.tabs.ActiveIdx() == i {
			style = activeTabStyle
		}
		rendered = append(rendered, style.Render(label))
	}

	bar := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
	return tabSeparatorStyle.Width(m.width).Render(bar)
}

func (m *MusicModel) renderContent() string {
	if m.pageLoading {
		return m.thumbList.WrapView(nil, m.spinner.View()+" Loading...")
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
			return m.thumbList.WrapView(nil, m.spinner.View()+" Loading...")
		}
		switch tab.kind {
		case musicTabArtist:
			return m.renderArtistPage(tab)
		case musicTabAlbum:
			return m.thumbList.WrapView(nil, m.renderAlbumPage(tab))
		case musicTabSong:
			return m.thumbList.WrapView(nil, tab.songDetail.View())
		}
	}
	return ""
}

func (m *MusicModel) renderHome() string {
	if m.homeLoading {
		return m.thumbList.WrapView(nil, m.spinner.View()+" Loading home...")
	}
	if !m.homeLoaded || len(m.homeSubs) == 0 {
		return styles.Dim.Render("Home feed not loaded")
	}

	subBar := renderMusicSubTabBar(m.homeSubs, m.homeSubIdx)

	ch := m.contentHeight()
	overhead := lipgloss.Height(subBar)
	if m.homeSubIdx < len(m.homeSubs) {
		m.homeSubs[m.homeSubIdx].list.SetSize(m.width, ch-overhead)
	}

	activeList := ""
	var visibleItems []list.Item
	if m.homeSubIdx < len(m.homeSubs) {
		activeList = m.homeSubs[m.homeSubIdx].list.View()
		visibleItems = shared.VisibleItems(m.homeSubs[m.homeSubIdx].list)
	}
	view := lipgloss.JoinVertical(lipgloss.Left, subBar, activeList)
	return m.thumbList.WrapView(visibleItems, view)
}

func (m *MusicModel) renderLibrary() string {
	if m.libraryLoading {
		return m.thumbList.WrapView(nil, m.spinner.View()+" Loading library...")
	}
	if !m.libraryLoaded || len(m.librarySubs) == 0 {
		return styles.Dim.Render("Press 'a' to authenticate to view library")
	}

	subBar := renderMusicSubTabBar(m.librarySubs, m.librarySubIdx)

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
	var visibleItems []list.Item
	if m.librarySubIdx < len(m.librarySubs) {
		activeList = m.librarySubs[m.librarySubIdx].list.View()
		visibleItems = shared.VisibleItems(m.librarySubs[m.librarySubIdx].list)
	}

	var sections []string
	sections = append(sections, subBar, activeList)
	if hint != "" {
		sections = append(sections, hint)
	}
	view := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return m.thumbList.WrapView(visibleItems, view)
}

func (m *MusicModel) renderSearch() string {
	return m.search.View()
}

func renderMusicSubTabBar(subs []subTab, activeIdx int) string {
	names := make([]string, len(subs))
	for i, sub := range subs {
		names[i] = sub.title
	}
	return shared.RenderSubTabBar(names, activeIdx)
}

// artistAboutView renders the About sub-tab body for a music artist page.
func artistAboutView(p *youtube.MusicArtistPage, width int) string {
	if p == nil {
		return styles.Dim.Render("(no artist data)")
	}
	var meta []string
	if p.SubscriberCount != "" {
		meta = append(meta, p.SubscriberCount)
	}
	return shared.AboutView(shared.AboutData{
		Name:            p.Name,
		MetaParts:       meta,
		Description:     p.Description,
		Subscribed:      p.Subscribed,
		SubscribedKnown: p.SubscribedKnown,
	}, width)
}

func (m *MusicModel) renderArtistPage(tab *musicTab) string {
	if len(tab.artistSubs) == 0 {
		return styles.Dim.Render("No content")
	}

	subBar := renderMusicSubTabBar(tab.artistSubs, tab.activeSubTab)

	if tab.activeSubTab < len(tab.artistSubs) && tab.artistSubs[tab.activeSubTab].isAbout {
		body := artistAboutView(tab.artistPage, m.width)
		view := lipgloss.JoinVertical(lipgloss.Left, subBar, body)
		return m.thumbList.WrapView(nil, view)
	}

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
	var visibleItems []list.Item
	if tab.activeSubTab < len(tab.artistSubs) {
		activeList = tab.artistSubs[tab.activeSubTab].list.View()
		visibleItems = shared.VisibleItems(tab.artistSubs[tab.activeSubTab].list)
	}

	var sections []string
	sections = append(sections, subBar, activeList)
	if hint != "" {
		sections = append(sections, hint)
	}
	view := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return m.thumbList.WrapView(visibleItems, view)
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
		statusBarStyle.Render(m.help.View(musicHelpAdapter{m.keys})),
		m.status.Msg != "")
}

func (m *MusicModel) resizeViews() {
	if m.width == 0 {
		return
	}
	ch := m.contentHeight()
	m.search.SetSize(m.width, ch)
	// Resize song detail tabs
	for i := range m.tabs.All() {
		if tab := m.tabs.At(i); tab.kind == musicTabSong {
			tab.songDetail.SetSize(m.width, ch)
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
	cmd := TryAuthenticate(&m.authenticating, m.client.IsAuthenticated(), &m.status, m.cfg.Auth.Browser)
	if cmd != nil {
		m.resizeViews()
	}
	return cmd
}

type musicItemSelectedMsg struct {
	item youtube.MusicItem
}

func (m *MusicModel) musicListDelegate() list.ItemDelegate {
	thumbH := m.cfg.Thumbnails.Height
	if thumbH <= 0 {
		thumbH = 5
	}
	return newMusicDelegate(m.thumbList.Renderer(), thumbH)
}

func newMusicSearchConfig(client youtube.MusicAPI, thumbList *shared.ThumbList, thumbH int) search.Config {
	var delegate list.ItemDelegate
	if imgR := thumbList.Renderer(); imgR != nil {
		delegate = newMusicDelegate(imgR, thumbH)
	} else {
		delegate = musicDelegate{}
	}
	return search.Config{
		Placeholder: "Search YouTube Music...",
		Delegate:    delegate,
		ThumbList:   thumbList,
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
	cmd := m.status.Set(msg, clearAfter)
	m.resizeViews()
	return cmd
}
