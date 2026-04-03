package app

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/auth"
	"github.com/deathmaz/ytui/internal/config"
	"github.com/deathmaz/ytui/internal/download"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/player"
	"github.com/deathmaz/ytui/internal/ui/detail"
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
type authResultMsg struct{ err error }
type authSuccessMsg struct{ client youtube.Client }

const maxVideoTabs = 6

// videoTab holds the state for a single video tab.
type videoTab struct {
	videoID string
	title   string
	detail  detail.Model
	formats []player.Format // cached quality list from yt-dlp
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
	picker        picker.Model
	urlInput      urlinput.Model
	cfg           *config.Config
	videoTabs     []videoTab
	activeTabIdx  int // index into videoTabs for current video tab

	pendingVideoURL string
	pendingOpen     *youtube.ParsedURL
	statusMsg       string
	statusSeq       int
	downloading     bool
	authenticating  bool
}

// Options holds startup options from command-line flags.
type Options struct {
	SearchQuery string
	OpenURL     *youtube.ParsedURL
}

// New creates a new root model with the given YouTube client, config, and options.
func New(client youtube.Client, cfg *config.Config, opts Options) *Model {
	h := help.New()
	h.ShortSeparator = "  "
	imgR := ytimage.NewRenderer()
	s := search.New(newVideoSearchConfig(client))
	if opts.SearchQuery != "" {
		s.SetQuery(opts.SearchQuery)
	}
	return &Model{
		activeView:  ViewSearch,
		keys:        DefaultKeyMap(),
		help:        h,
		search:      s,
		feed:        feed.New(client),
		subs:        subs.New(client),
		imgR:        imgR,
		ytClient:    client,
		picker:      picker.New(),
		urlInput:    urlinput.New(),
		cfg:         cfg,
		pendingOpen: opts.OpenURL,
	}
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
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
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

		if key.Matches(msg, m.keys.ForceQuit) {
			return m, tea.Quit
		}

		if searchFocusedCmd, ok := handleSearchFocused(msg, &m.search, m.activeView == ViewSearch, m.keys); ok {
			return m, searchFocusedCmd
		}

		// Esc handling
		if key.Matches(msg, m.keys.Back) {
			if m.activeView == ViewVideoTab {
				m.closeActiveVideoTab()
				if len(m.videoTabs) > 0 {
					m.activeTabIdx = len(m.videoTabs) - 1
				} else {
					m.activeView = ViewSearch
				}
				return m, nil
			}
		}

		switch {
			case key.Matches(msg, m.keys.Quit):
				return m, tea.Quit
			case key.Matches(msg, m.keys.Help):
				m.help.ShowAll = !m.help.ShowAll
				return m, nil
			case key.Matches(msg, m.keys.Feed):
				m.switchTo(ViewFeed)
				return m, m.feed.Load(false)
			case key.Matches(msg, m.keys.Subs):
				m.switchTo(ViewSubs)
				return m, m.subs.Load(false)
			case key.Matches(msg, m.keys.Search):
				m.switchTo(ViewSearch)
				m.search.Focus()
				return m, nil
			case key.Matches(msg, m.keys.Detail):
				return m, m.openDetail()
			case key.Matches(msg, m.keys.Play):
				return m, m.quickPlay()
			case key.Matches(msg, m.keys.PlayPick):
				return m, m.fetchFormatsAndPlay()
			case key.Matches(msg, m.keys.Download):
				return m, m.startDownload()
			case key.Matches(msg, m.keys.Auth):
				return m, m.authenticate()
			case key.Matches(msg, m.keys.Open):
				return m, m.openInBrowser()
			case key.Matches(msg, m.keys.OpenURL):
				return m, m.urlInput.Show(m.width, m.height)
			case key.Matches(msg, m.keys.Yank):
				return m, m.copyURL()
			case key.Matches(msg, m.keys.Refresh):
				return m, m.refresh()
			}

			// Video tab number keys (4-9)
		if k := msg.String(); len(k) == 1 && k[0] >= '4' && k[0] <= '9' {
			idx := int(k[0] - '4')
			if idx < len(m.videoTabs) {
				m.activeView = ViewVideoTab
				m.activeTabIdx = idx
				return m, nil
			}
		}

	case shared.VideoSelectedMsg:
		return m, m.openVideoTab(&msg.Video)

	case urlinput.SubmitMsg:
		return m, m.openParsedURL(&msg.Parsed)

	case urlinput.CancelMsg:

	case subs.ChannelSelectedMsg:
		// TODO: show channel videos
		return m, m.setStatus("Channel: "+msg.Channel.Name, 3*time.Second)

	case formatsLoadedMsg:
		var formats []player.Format
		if msg.err != nil {
			formats = player.DefaultFormats()
		} else {
			formats = msg.formats
		}
		// Cache on active video tab
		if tab := m.activeTab(); tab != nil {
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

	case authSuccessMsg:
		m.authenticating = false
		m.ytClient = msg.client
		m.search = search.New(newVideoSearchConfig(msg.client))
		m.feed = feed.New(msg.client)
		m.subs = subs.New(msg.client)
		m.resizeViews()
		feedCmd := m.feed.Load(true)
		subsCmd := m.subs.Load(true)
		var openCmd tea.Cmd
		if m.pendingOpen != nil {
			openCmd = m.openParsedURL(m.pendingOpen)
			m.pendingOpen = nil
		}
		return m, tea.Batch(m.setStatus("Authenticated via "+m.cfg.Auth.Browser, 3*time.Second), feedCmd, subsCmd, openCmd)

	case authResultMsg:
		m.authenticating = false
		return m, m.setStatus("Auth failed: "+msg.err.Error(), 5*time.Second)

	case clearStatusMsg:
		if handleClearStatus(msg, m.statusSeq, &m.statusMsg) {
			m.resizeViews()
		}
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
	case ViewVideoTab:
		if tab := m.activeTab(); tab != nil {
			if vlm, ok := msg.(detail.VideoLoadedMsg); ok && vlm.Err == nil && vlm.Video != nil && tab.title == "" {
				tab.title = vlm.Video.Title
			}
			var cmd tea.Cmd
			tab.detail, cmd = tab.detail.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	if m.urlInput.IsActive() {
		return m.urlInput.View()
	}

	if m.picker.IsActive() {
		return m.picker.View()
	}

	var statusLine string
	if m.statusMsg != "" {
		statusLine = styles.Accent.Render(m.statusMsg)
	} else if m.downloading {
		statusLine = styles.Dim.Render("Downloading...")
	}

	return composeSections(
		m.renderTabs(),
		m.renderContent(),
		statusLine,
		statusBarStyle.Render(m.help.View(m.keys)),
	)
}

// activeTab returns the currently active video tab, or nil.
func (m *Model) activeTab() *videoTab {
	if m.activeView != ViewVideoTab || m.activeTabIdx >= len(m.videoTabs) {
		return nil
	}
	return &m.videoTabs[m.activeTabIdx]
}

// openVideoTab opens a new video tab or switches to existing one for the same video.
func (m *Model) openParsedURL(p *youtube.ParsedURL) tea.Cmd {
	if p == nil {
		return nil
	}
	switch p.Kind {
	case youtube.URLVideo:
		return m.openVideoTab(&youtube.Video{ID: p.ID})
	}
	return nil
}

func (m *Model) openVideoTab(v *youtube.Video) tea.Cmd {
	// Check if already open
	for i, tab := range m.videoTabs {
		if tab.videoID == v.ID {
			m.activeView = ViewVideoTab
			m.activeTabIdx = i
			return nil
		}
	}

	// Create new tab
	if len(m.videoTabs) >= maxVideoTabs {
		return m.setStatus("Max video tabs reached (close one with Esc)", 3*time.Second)
	}

	d := detail.New(m.ytClient, m.imgR)
	d.SetSize(m.width, m.contentHeight())

	m.videoTabs = append(m.videoTabs, videoTab{
		videoID: v.ID,
		title:   v.Title,
		detail:  d,
	})
	m.activeTabIdx = len(m.videoTabs) - 1
	m.activeView = ViewVideoTab
	return m.videoTabs[m.activeTabIdx].detail.LoadVideo(v.ID)
}

// closeActiveVideoTab closes the current video tab.
func (m *Model) closeActiveVideoTab() {
	if m.activeView != ViewVideoTab || len(m.videoTabs) == 0 {
		return
	}
	m.videoTabs = append(m.videoTabs[:m.activeTabIdx], m.videoTabs[m.activeTabIdx+1:]...)
	if m.activeTabIdx >= len(m.videoTabs) {
		m.activeTabIdx = len(m.videoTabs) - 1
	}
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
	case ViewVideoTab:
		if tab := m.activeTab(); tab != nil {
			return tab.detail.Video()
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
	if tab := m.activeTab(); tab != nil && len(tab.formats) > 0 {
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
	m.statusMsg = "Downloading: " + v.Title
	url := v.URL
	dlCfg := m.cfg.Download
	return func() tea.Msg {
		result := download.Download(url, dlCfg.Format, dlCfg.OutputDir, dlCfg.Command)
		return downloadResultMsg{result: result}
	}
}

func (m *Model) setStatus(msg string, clearAfter time.Duration) tea.Cmd {
	cmd := setStatusCmd(&m.statusSeq, &m.statusMsg, msg, clearAfter)
	m.resizeViews()
	return cmd
}

func (m *Model) authenticate() tea.Cmd {
	if m.authenticating {
		return nil
	}
	if m.ytClient.IsAuthenticated() {
		return m.setStatus("Already authenticated", 3*time.Second)
	}
	m.authenticating = true
	m.statusMsg = "Authenticating via " + m.cfg.Auth.Browser + "..."
	return func() tea.Msg {
		jar, err := auth.ExtractCookies(context.Background(), m.cfg.Auth.Browser)
		if err != nil {
			return authResultMsg{err: err}
		}
		httpClient := auth.HTTPClient(jar)
		newClient, err := youtube.NewInnerTubeClient(httpClient)
		if err != nil {
			return authResultMsg{err: err}
		}
		return authSuccessMsg{client: newClient}
	}
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
		m.statusMsg != "" || m.downloading)
}

func (m *Model) resizeViews() {
	if m.width == 0 {
		return
	}
	ch := m.contentHeight()
	m.search.SetSize(m.width, ch)
	m.feed.SetSize(m.width, ch)
	m.subs.SetSize(m.width, ch)
	for i := range m.videoTabs {
		m.videoTabs[i].detail.SetSize(m.width, ch)
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

	// Video tabs
	for i, tab := range m.videoTabs {
		title := tab.title
		if title == "" {
			title = "..."
		}
		label := fmt.Sprintf("[%d] %s", i+4, shared.Truncate(title, 20))
		style := tabStyle
		if m.activeView == ViewVideoTab && m.activeTabIdx == i {
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
	case ViewVideoTab:
		if tab := m.activeTab(); tab != nil {
			return tab.detail.View()
		}
	}
	return ""
}

func newVideoSearchConfig(client youtube.Client) search.Config {
	return search.Config{
		Placeholder: "Search YouTube...",
		Delegate:    shared.VideoDelegate{},
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
