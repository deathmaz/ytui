package app

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/config"
	"github.com/deathmaz/ytui/internal/download"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/player"
	"github.com/deathmaz/ytui/internal/ui/channel"
	"github.com/deathmaz/ytui/internal/ui/detail"
	"github.com/deathmaz/ytui/internal/ui/playlist"
	"github.com/deathmaz/ytui/internal/ui/post"
	"github.com/deathmaz/ytui/internal/ui/feed"
	"github.com/deathmaz/ytui/internal/ui/picker"
	"github.com/deathmaz/ytui/internal/ui/search"
	"github.com/deathmaz/ytui/internal/ui/urlinput"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/subs"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

var (
	tabStyle          = styles.Tab
	activeTabStyle    = styles.ActiveTab
	statusBarStyle    = styles.StatusBar
	tabSeparatorStyle = styles.TabSeparator
)

type playerErrorMsg struct{ err error }
type formatsLoadedMsg struct {
	url     string
	formats []player.Format
	err     error
}
type downloadResultMsg struct{ result download.Result }
type clearStatusMsg struct{ seq int }

const maxDynamicTabs = 6

type tabKind int

const (
	tabVideo    tabKind = iota
	tabChannel
	tabPlaylist
	tabPost
)

// dynamicTab holds the state for a single dynamic tab (video detail or channel).
type dynamicTab struct {
	kind    tabKind
	id      string // videoID or channelID — used for deduplication
	title   string
	detail   detail.Model    // video tab
	formats  []player.Format // cached quality list (video tab only)
	channel  channel.Model   // channel tab
	playlist playlist.Model  // playlist tab
	post     post.Model      // post tab
}

// Model is the root Bubble Tea model.
type Model struct {
	activeView    View
	width         int
	height        int
	keys          KeyMap
	help          help.Model
	search        search.Model
	feed          feed.Model
	subs          subs.Model
	ytClient      youtube.Client
	imgR          *ytimage.Renderer
	listThumbList *shared.ThumbList
	listDelegate  list.ItemDelegate
	picker        picker.Model
	urlInput      urlinput.Model
	cfg           *config.Config
	tabs TabSet[dynamicTab]

	pendingVideoURL string
	pendingOpen     *youtube.ParsedURL
	startupWarning  string
	status          StatusManager
	downloading     bool
	authenticating  bool
}

// Options holds startup options from command-line flags.
type Options struct {
	SearchQuery string
	OpenURL     *youtube.ParsedURL
	Warning     string
}

// New creates a new root model with the given YouTube client, config, and options.
func New(client youtube.Client, cfg *config.Config, opts Options) *Model {
	h := help.New()
	h.ShortSeparator = "  "
	imgR := ytimage.NewRenderer()
	thumbList, listDelegate := newVideoListSetup(cfg)
	s := search.New(newVideoSearchConfig(client, thumbList, listDelegate))
	if opts.SearchQuery != "" {
		s.SetQuery(opts.SearchQuery)
	}
	return &Model{
		activeView:  ViewSearch,
		keys:        DefaultKeyMap(),
		help:        h,
		search:      s,
		feed:        feed.New(client, listDelegate, thumbList),
		subs:        subs.New(client),
		imgR:          imgR,
		listThumbList: thumbList,
		listDelegate:  listDelegate,
		ytClient:      client,
		picker:      picker.New(),
		urlInput:    urlinput.New(),
		cfg:         cfg,
		tabs:           NewTabSet[dynamicTab](maxDynamicTabs, func(t *dynamicTab) string { return t.id }),
		pendingOpen:    opts.OpenURL,
		startupWarning: opts.Warning,
	}
}

// newVideoListSetup creates the shared thumbnail infrastructure for video-mode
// lists (search and feed). Returns nil ThumbList and plain delegate when
// thumbnails are disabled.
func newVideoListSetup(cfg *config.Config) (*shared.ThumbList, list.ItemDelegate) {
	if !cfg.Thumbnails.Enabled {
		return nil, shared.VideoDelegate{}
	}
	thumbH := cfg.Thumbnails.Height
	if thumbH <= 0 {
		thumbH = 5
	}
	imgR := ytimage.NewRenderer()
	thumbList := shared.NewThumbList(imgR, shared.VideoThumbURL)
	delegate := shared.NewThumbDelegate(imgR, thumbH, shared.VideoThumbURL, shared.RenderVideoText)
	return thumbList, delegate
}

func (m *Model) Init() tea.Cmd {
	cmd := initCmds(
		m.cfg.Auth.AuthOnStartup,
		&m.pendingOpen,
		m.search.Init(),
		m.authenticate,
		m.openParsedURL,
		m.search.Query(),
		m.search.Refresh,
	)
	if m.startupWarning != "" {
		cmd = tea.Batch(cmd, m.setStatus(m.startupWarning, 10*time.Second))
		m.startupWarning = ""
	}
	// Auto-load the active view on startup
	switch m.activeView {
	case ViewFeed:
		return tea.Batch(cmd, m.feed.Load(false))
	case ViewSubs:
		return tea.Batch(cmd, m.subs.Load(false))
	}
	return cmd
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		HandleWindowSize(msg, &m.width, &m.height, &m.help)
		m.resizeViews()

	case tea.KeyMsg:
		// Modal dialogs take priority
		if m.picker.IsActive() {
			var cmd tea.Cmd
			m.picker, cmd = m.picker.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}
		if cmd, handled := handleURLInput(msg, &m.urlInput); handled {
			return m, cmd
		}

		if searchFocusedCmd, ok := handleSearchFocused(msg, &m.search, m.activeView == ViewSearch, m.keys); ok {
			return m, searchFocusedCmd
		}

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
				m.switchTo(ViewSearch)
				m.search.Focus()
				return m, nil
			}
		}

		// Esc handling
		if key.Matches(msg, m.keys.Back) {
			if m.activeView == ViewDynamicTab {
				m.closeActiveTab()
				if m.tabs.Len() == 0 {
					m.activeView = ViewSearch
				}
				return m, nil
			}
		}

		// Video-specific keys
		switch {
			case key.Matches(msg, m.keys.Feed):
				m.switchTo(ViewFeed)
				return m, m.feed.Load(false)
			case key.Matches(msg, m.keys.Subs):
				m.switchTo(ViewSubs)
				return m, m.subs.Load(false)
			case key.Matches(msg, m.keys.Detail):
				return m, m.openDetail()
			case key.Matches(msg, m.keys.Play):
				return m, m.quickPlay()
			case key.Matches(msg, m.keys.PlayPick):
				return m, m.fetchFormatsAndPlay()
			case key.Matches(msg, m.keys.Download):
				return m, m.startDownload()
			case key.Matches(msg, m.keys.Open):
				return m, m.openInBrowser()
			case key.Matches(msg, m.keys.Yank):
				return m, m.copyURL()
			case key.Matches(msg, m.keys.Refresh):
				return m, m.refresh()
			case key.Matches(msg, m.keys.Channel):
				return m, m.openChannelForSelected()
			}

			// Dynamic tab number keys (4-9)
		if k := msg.String(); len(k) == 1 && k[0] >= '4' && k[0] <= '9' {
			idx := int(k[0] - '4')
			if idx < m.tabs.Len() {
				m.activeView = ViewDynamicTab
				m.tabs.SetActive(idx)
				return m, nil
			}
		}

	case shared.VideoSelectedMsg:
		return m, m.openVideoTab(&msg.Video)

	case urlinput.SubmitMsg:
		return m, m.openParsedURL(&msg.Parsed)

	case urlinput.CancelMsg:

	case subs.ChannelSelectedMsg:
		return m, m.openChannelTab(msg.Channel)

	case channel.PlaylistSelectedMsg:
		return m, m.openPlaylistTab(msg.Playlist)

	case channel.PostSelectedMsg:
		return m, m.openPostTab(msg.Post)

	case formatsLoadedMsg:
		var formats []player.Format
		if msg.err != nil {
			formats = player.DefaultFormats()
		} else {
			formats = msg.formats
		}
		// Cache on active video tab
		if tab := m.activeVideoTab(); tab != nil {
			tab.formats = formats
		}
		m.picker.Show(formats, m.width, m.height)
		m.pendingVideoURL = msg.url
		return m, nil

	case picker.SelectedMsg:
		if m.pendingVideoURL != "" {
			url := m.pendingVideoURL
			quality := msg.Format.ID
			m.pendingVideoURL = ""
			return m, playVideoCmd(url, quality, m.cfg.Player.Video.Command, m.cfg.Player.Video.Args)
		}
		return m, nil

	case picker.CancelledMsg:
		m.pendingVideoURL = ""
		return m, nil

	case playerErrorMsg:
		return m, m.setStatus("Player error: "+msg.err.Error(), 5*time.Second)

	case downloadResultMsg:
		m.downloading = false
		if msg.result.Err != nil {
			return m, m.setStatus("Download failed: "+msg.result.Err.Error(), 5*time.Second)
		}
		return m, m.setStatus("Downloaded: "+msg.result.Title, 5*time.Second)

	case AuthResult:
		return m, HandleAuthResult(msg, &m.authenticating, &m.status, m.cfg.Auth.Browser,
			&m.pendingOpen, m.openParsedURL,
			func(httpClient *http.Client) error {
				newClient, err := youtube.NewInnerTubeClient(httpClient)
				if err != nil {
					return err
				}
				m.ytClient = newClient
				tl, dl := newVideoListSetup(m.cfg)
				m.listThumbList = tl
				m.listDelegate = dl
				m.search = search.New(newVideoSearchConfig(newClient, m.listThumbList, dl))
				m.feed = feed.New(newClient, dl, m.listThumbList)
				m.subs = subs.New(newClient)
				return nil
			},
			func() tea.Cmd {
				switch m.activeView {
				case ViewFeed:
					return m.feed.Load(true)
				case ViewSubs:
					return m.subs.Load(true)
				}
				return nil
			},
			m.resizeViews,
		)

	case clearStatusMsg:
		if m.status.HandleClear(msg) {
			m.resizeViews()
		}

	case ytimage.ThumbnailLoadedMsg:
		// Always route list thumbnail messages to the shared ThumbList,
		// regardless of which view is active. This ensures fetches that
		// complete while a video detail tab is active still get cached.
		m.listThumbList.HandleMsg(msg)
	}

	// Delegate to active view
	switch m.activeView {
	case ViewSearch:
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		cmds = append(cmds, cmd)
	case ViewFeed:
		var cmd tea.Cmd
		m.feed, cmd = m.feed.Update(msg)
		cmds = append(cmds, cmd)
	case ViewSubs:
		var cmd tea.Cmd
		m.subs, cmd = m.subs.Update(msg)
		cmds = append(cmds, cmd)
	case ViewDynamicTab:
		if tab := m.tabs.Active(); tab != nil {
			var cmd tea.Cmd
			switch tab.kind {
			case tabVideo:
				if vlm, ok := msg.(detail.VideoLoadedMsg); ok && vlm.Err == nil && vlm.Video != nil && tab.title == "" {
					tab.title = vlm.Video.Title
				}
				tab.detail, cmd = tab.detail.Update(msg)
			case tabChannel:
				tab.channel, cmd = tab.channel.Update(msg)
			case tabPlaylist:
				tab.playlist, cmd = tab.playlist.Update(msg)
			case tabPost:
				tab.post, cmd = tab.post.Update(msg)
			}
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	var statusLine string
	if m.status.Msg != "" {
		statusLine = styles.Accent.Render(m.status.Msg)
	} else if m.downloading {
		statusLine = styles.Dim.Render("Downloading...")
	}

	return RenderShell(
		m.width,
		[]ModalView{&m.urlInput, &m.picker},
		m.renderTabs,
		m.renderContent,
		statusLine,
		statusBarStyle.Render(m.help.View(m.keys)),
	)
}

// activeVideoTab returns the currently active video tab, or nil.
func (m *Model) activeVideoTab() *dynamicTab {
	if m.activeView != ViewDynamicTab {
		return nil
	}
	tab := m.tabs.Active()
	if tab != nil && tab.kind == tabVideo {
		return tab
	}
	return nil
}

func (m *Model) openParsedURL(p *youtube.ParsedURL) tea.Cmd {
	if p == nil {
		return nil
	}
	switch p.Kind {
	case youtube.URLVideo:
		return m.openVideoTab(&youtube.Video{ID: p.ID})
	case youtube.URLChannel:
		return m.openChannelTab(youtube.Channel{ID: p.ID})
	case youtube.URLPlaylist:
		return m.openPlaylistTab(youtube.Playlist{ID: p.ID})
	}
	return nil
}

func (m *Model) openVideoTab(v *youtube.Video) tea.Cmd {
	if idx, found := m.tabs.Find(v.ID); found {
		m.activeView = ViewDynamicTab
		m.tabs.SetActive(idx)
		return nil
	}

	d := detail.New(m.ytClient, m.imgR)
	d.SetSize(m.width, m.contentHeight())

	idx, err := m.tabs.Open(dynamicTab{
		kind:  tabVideo,
		id:    v.ID,
		title: v.Title,
		detail: d,
	})
	if err != nil {
		return m.setStatus("Max tabs reached (close one with Esc)", 3*time.Second)
	}
	m.tabs.SetActive(idx)
	m.activeView = ViewDynamicTab
	return m.tabs.Active().detail.LoadVideo(v.ID)
}

func (m *Model) openChannelTab(ch youtube.Channel) tea.Cmd {
	if idx, found := m.tabs.Find(ch.ID); found {
		m.activeView = ViewDynamicTab
		m.tabs.SetActive(idx)
		return nil
	}

	cv := channel.New(m.ytClient, m.listDelegate, m.listThumbList, m.cfg.Thumbnails)
	cv.SetSize(m.width, m.contentHeight())

	title := ch.Name
	if title == "" {
		title = ch.Handle
	}
	idx, err := m.tabs.Open(dynamicTab{
		kind:    tabChannel,
		id:      ch.ID,
		title:   title,
		channel: cv,
	})
	if err != nil {
		return m.setStatus("Max tabs reached (close one with Esc)", 3*time.Second)
	}
	m.tabs.SetActive(idx)
	m.activeView = ViewDynamicTab
	return m.tabs.Active().channel.Load(ch)
}

func (m *Model) openPlaylistTab(pl youtube.Playlist) tea.Cmd {
	if idx, found := m.tabs.Find(pl.ID); found {
		m.activeView = ViewDynamicTab
		m.tabs.SetActive(idx)
		return nil
	}

	pv := playlist.New(m.ytClient, m.listDelegate, m.listThumbList)
	pv.SetSize(m.width, m.contentHeight())

	title := pl.Title
	if title == "" {
		title = pl.ID
	}
	idx, err := m.tabs.Open(dynamicTab{
		kind:     tabPlaylist,
		id:       pl.ID,
		title:    title,
		playlist: pv,
	})
	if err != nil {
		return m.setStatus("Max tabs reached (close one with Esc)", 3*time.Second)
	}
	m.tabs.SetActive(idx)
	m.activeView = ViewDynamicTab
	return m.tabs.Active().playlist.Load(pl)
}

func (m *Model) openPostTab(p youtube.Post) tea.Cmd {
	if idx, found := m.tabs.Find(p.ID); found {
		m.activeView = ViewDynamicTab
		m.tabs.SetActive(idx)
		return nil
	}

	pv := post.New(m.ytClient, m.imgR)
	pv.SetSize(m.width, m.contentHeight())

	title := shared.Truncate(p.Content, 30)
	if title == "" {
		title = p.ID
	}
	idx, err := m.tabs.Open(dynamicTab{
		kind:  tabPost,
		id:    p.ID,
		title: title,
		post:  pv,
	})
	if err != nil {
		return m.setStatus("Max tabs reached (close one with Esc)", 3*time.Second)
	}
	m.tabs.SetActive(idx)
	m.activeView = ViewDynamicTab
	return m.tabs.Active().post.Load(p)
}

func (m *Model) openChannelForSelected() tea.Cmd {
	v := m.selectedVideo()
	if v == nil || v.ChannelID == "" {
		return nil
	}
	return m.openChannelTab(youtube.Channel{
		ID:   v.ChannelID,
		Name: v.ChannelName,
	})
}

// closeActiveTab closes the current dynamic tab.
func (m *Model) closeActiveTab() {
	if m.activeView != ViewDynamicTab || m.tabs.Len() == 0 {
		return
	}
	m.tabs.Close(m.tabs.ActiveIdx())
}

func (m *Model) switchTo(v View) {
	m.activeView = v
}

func (m *Model) selectedVideo() *youtube.Video {
	switch m.activeView {
	case ViewSearch:
		if item, ok := m.search.SelectedItem().(shared.VideoItem); ok {
			return &item.Video
		}
	case ViewFeed:
		if v, ok := m.feed.SelectedVideo(); ok {
			return &v
		}
	case ViewDynamicTab:
		if tab := m.tabs.Active(); tab != nil {
			switch tab.kind {
			case tabVideo:
				return tab.detail.Video()
			case tabChannel:
				return tab.channel.SelectedVideo()
			case tabPlaylist:
				return tab.playlist.SelectedVideo()
			}
		}
	}
	return nil
}

func (m *Model) openDetail() tea.Cmd {
	v := m.selectedVideo()
	if v == nil {
		return nil
	}
	return m.openVideoTab(v)
}

func (m *Model) quickPlay() tea.Cmd {
	v := m.selectedVideo()
	if v == nil {
		return nil
	}
	return playVideoCmd(v.URL, m.cfg.Player.Video.Quality, m.cfg.Player.Video.Command, m.cfg.Player.Video.Args)
}

func (m *Model) fetchFormatsAndPlay() tea.Cmd {
	v := m.selectedVideo()
	if v == nil {
		return nil
	}
	// Use cached formats from the active video tab if available
	if tab := m.activeVideoTab(); tab != nil && len(tab.formats) > 0 {
		m.picker.Show(tab.formats, m.width, m.height)
		m.pendingVideoURL = v.URL
		return nil
	}
	url := v.URL
	dlCmd := m.cfg.Download.Command
	return tea.Batch(
		m.setStatus("Fetching available qualities...", 10*time.Second),
		func() tea.Msg {
			formats, err := player.FetchFormats(context.Background(), url, dlCmd)
			return formatsLoadedMsg{url: url, formats: formats, err: err}
		},
	)
}

func (m *Model) startDownload() tea.Cmd {
	v := m.selectedVideo()
	if v == nil || m.downloading {
		return nil
	}
	m.downloading = true
	m.status.SetPermanent("Downloading: " + v.Title)
	url := v.URL
	dlCfg := m.cfg.Download
	return func() tea.Msg {
		result := download.Download(url, dlCfg.Format, dlCfg.OutputDir, dlCfg.Command)
		return downloadResultMsg{result: result}
	}
}

func (m *Model) setStatus(msg string, clearAfter time.Duration) tea.Cmd {
	cmd := m.status.Set(msg, clearAfter)
	m.resizeViews()
	return cmd
}

func (m *Model) authenticate() tea.Cmd {
	cmd := TryAuthenticate(&m.authenticating, m.ytClient.IsAuthenticated(), &m.status, m.cfg.Auth.Browser)
	if cmd != nil {
		m.resizeViews()
	}
	return cmd
}

func (m *Model) openInBrowser() tea.Cmd {
	v := m.selectedVideo()
	if v == nil {
		return nil
	}
	url := v.URL
	return tea.Batch(
		m.setStatus("Opening in browser...", 2*time.Second),
		func() tea.Msg {
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				cmd = exec.Command("open", url)
			case "windows":
				cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
			default:
				cmd = exec.Command("xdg-open", url)
			}
			cmd.Start()
			return nil
		},
	)
}

func (m *Model) copyURL() tea.Cmd {
	v := m.selectedVideo()
	if v == nil {
		return nil
	}
	url := v.URL
	return tea.Batch(
		m.setStatus("URL copied: "+url, 3*time.Second),
		func() tea.Msg {
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				cmd = exec.Command("pbcopy")
			case "windows":
				cmd = exec.Command("clip")
			default:
				cmd = exec.Command("xclip", "-selection", "clipboard")
			}
			cmd.Stdin = strings.NewReader(url)
			if err := cmd.Run(); err != nil {
				cmd = exec.Command("xsel", "--clipboard", "--input")
				cmd.Stdin = strings.NewReader(url)
				cmd.Run()
			}
			return nil
		},
	)
}

func (m *Model) refresh() tea.Cmd {
	switch m.activeView {
	case ViewFeed:
		return m.feed.Load(true)
	case ViewSubs:
		return m.subs.Load(true)
	case ViewSearch:
		if q := m.search.Query(); q != "" {
			return m.search.Refresh()
		}
	}
	return nil
}

func playVideoCmd(url, format, playerCmd string, playerArgs []string) tea.Cmd {
	return func() tea.Msg {
		if err := player.Play(url, format, playerCmd, playerArgs); err != nil {
			return playerErrorMsg{err: err}
		}
		return nil
	}
}

func (m *Model) contentHeight() int {
	return calcContentHeight(m.height, m.renderTabs(),
		statusBarStyle.Render(m.help.View(m.keys)),
		m.status.Msg != "" || m.downloading)
}

func (m *Model) resizeViews() {
	if m.width == 0 {
		return
	}
	ch := m.contentHeight()
	m.search.SetSize(m.width, ch)
	m.feed.SetSize(m.width, ch)
	m.subs.SetSize(m.width, ch)
	for i := range m.tabs.All() {
		tab := m.tabs.At(i)
		switch tab.kind {
		case tabVideo:
			tab.detail.SetSize(m.width, ch)
		case tabChannel:
			tab.channel.SetSize(m.width, ch)
		case tabPlaylist:
			tab.playlist.SetSize(m.width, ch)
		case tabPost:
			tab.post.SetSize(m.width, ch)
		}
	}
}

func (m *Model) renderTabs() string {
	var rendered []string

	// Fixed tabs
	fixedTabs := []struct {
		label string
		view  View
	}{
		{"[1] Feed", ViewFeed},
		{"[2] Subs", ViewSubs},
		{"[3] Search", ViewSearch},
	}
	for _, t := range fixedTabs {
		style := tabStyle
		if t.view == m.activeView {
			style = activeTabStyle
		}
		rendered = append(rendered, style.Render(t.label))
	}

	// Dynamic tabs
	for i, tab := range m.tabs.All() {
		title := tab.title
		if title == "" {
			title = "..."
		}
		prefix := ""
		switch tab.kind {
		case tabChannel:
			prefix = "@ "
		case tabPlaylist:
			prefix = "▶ "
		case tabPost:
			prefix = "✎ "
		}
		label := fmt.Sprintf("[%d] %s%s", i+4, prefix, shared.Truncate(title, 20-len(prefix)))
		style := tabStyle
		if m.activeView == ViewDynamicTab && m.tabs.ActiveIdx() == i {
			style = activeTabStyle
		}
		rendered = append(rendered, style.Render(label))
	}

	bar := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
	return tabSeparatorStyle.Width(m.width).Render(bar)
}


func (m *Model) renderContent() string {
	switch m.activeView {
	case ViewSearch:
		return m.search.View()
	case ViewFeed:
		return m.feed.View()
	case ViewSubs:
		return m.subs.View()
	case ViewDynamicTab:
		if tab := m.tabs.Active(); tab != nil {
			switch tab.kind {
			case tabVideo:
				return tab.detail.View()
			case tabChannel:
				return tab.channel.View()
			case tabPlaylist:
				return tab.playlist.View()
			case tabPost:
				return tab.post.View()
			}
		}
	}
	return ""
}

func newVideoSearchConfig(client youtube.Client, thumbList *shared.ThumbList, delegate list.ItemDelegate) search.Config {
	return search.Config{
		Placeholder: "Search YouTube...",
		Delegate:    delegate,
		ThumbList:   thumbList,
		SearchFn: func(ctx context.Context, query, pageToken string) search.SearchResult {
			page, err := client.Search(ctx, query, pageToken)
			if err != nil {
				return search.SearchResult{Err: err}
			}
			var items []list.Item
			for _, v := range page.Items {
				items = append(items, shared.VideoItem{Video: v})
			}
			return search.SearchResult{
				Items:     items,
				NextToken: page.NextToken,
			}
		},
		SelectFn: func(item list.Item) tea.Msg {
			if vi, ok := item.(shared.VideoItem); ok {
				return shared.VideoSelectedMsg{Video: vi.Video}
			}
			return nil
		},
	}
}
