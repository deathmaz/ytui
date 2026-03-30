package app

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/auth"
	"github.com/deathmaz/ytui/internal/config"
	"github.com/deathmaz/ytui/internal/download"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/player"
	"github.com/deathmaz/ytui/internal/ui/comments"
	"github.com/deathmaz/ytui/internal/ui/detail"
	"github.com/deathmaz/ytui/internal/ui/feed"
	"github.com/deathmaz/ytui/internal/ui/picker"
	"github.com/deathmaz/ytui/internal/ui/search"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/subs"
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
type downloadResultMsg struct{ result download.Result }
type clearStatusMsg struct{ seq int }
type authResultMsg struct{ err error }
type authSuccessMsg struct{ client youtube.Client }

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
	feed       feed.Model
	subs       subs.Model
	comments   comments.Model
	ytClient   youtube.Client
	imgR   *ytimage.Renderer
	picker picker.Model
	cfg    *config.Config

	pendingVideoURL string
	statusMsg       string
	statusSeq       int
	downloading     bool
	authenticating  bool
}

// New creates a new root model with the given YouTube client and config.
func New(client youtube.Client, cfg *config.Config) *Model {
	h := help.New()
	h.ShortSeparator = "  "
	imgR := ytimage.NewRenderer()
	return &Model{
		activeView: ViewSearch,
		keys:       DefaultKeyMap(),
		help:       h,
		search:     search.New(client),
		detail:     detail.New(client, imgR),
		feed:       feed.New(client),
		subs:       subs.New(client),
		comments:   comments.New(client),
		imgR:       imgR,
		ytClient:   client,
		picker:     picker.New(),
		cfg:        cfg,
	}
}

func (m *Model) Init() tea.Cmd {
	if m.cfg.Auth.AuthOnStartup {
		return tea.Batch(m.search.Init(), m.authenticate())
	}
	return m.search.Init()
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
				m.openQualityPicker()
				return m, nil
			case key.Matches(msg, m.keys.Comments):
				return m, m.openComments()
			case key.Matches(msg, m.keys.Download):
				return m, m.startDownload()
			case key.Matches(msg, m.keys.Auth):
				return m, m.authenticate()
			case key.Matches(msg, m.keys.Open):
				return m, m.openInBrowser()
			case key.Matches(msg, m.keys.Yank):
				return m, m.copyURL()
			case key.Matches(msg, m.keys.Refresh):
				return m, m.refresh()
			}
		}

	case shared.VideoSelectedMsg:
		m.switchTo(ViewDetail)
		return m, m.detail.LoadVideo(msg.Video.ID)

	case subs.ChannelSelectedMsg:
		// TODO: show channel videos
		return m, m.setStatus("Channel: "+msg.Channel.Name, 3*time.Second)

	case picker.SelectedMsg:
		if m.pendingVideoURL != "" {
			url := m.pendingVideoURL
			format := msg.Format.ID
			m.pendingVideoURL = ""
			return m, playVideoCmd(url, format, m.cfg.Player.Command, m.cfg.Player.Args)
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
		m.search = search.New(msg.client)
		m.detail = detail.New(msg.client, m.imgR)
		m.feed = feed.New(msg.client)
		m.subs = subs.New(msg.client)
		m.comments = comments.New(msg.client)
		m.resizeViews()
		return m, m.setStatus("Authenticated via "+m.cfg.Auth.Browser, 3*time.Second)

	case authResultMsg:
		m.authenticating = false
		return m, m.setStatus("Auth failed: "+msg.err.Error(), 5*time.Second)

	case clearStatusMsg:
		if msg.seq == m.statusSeq {
			m.statusMsg = ""
			m.resizeViews()
		}
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
	case ViewFeed:
		var cmd tea.Cmd
		m.feed, cmd = m.feed.Update(msg)
		cmds = append(cmds, cmd)
	case ViewSubs:
		var cmd tea.Cmd
		m.subs, cmd = m.subs.Update(msg)
		cmds = append(cmds, cmd)
	case ViewComments:
		var cmd tea.Cmd
		m.comments, cmd = m.comments.Update(msg)
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

	var statusLine string
	if m.statusMsg != "" {
		statusLine = styles.Accent.Render(m.statusMsg)
	} else if m.downloading {
		statusLine = styles.Dim.Render("Downloading...")
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
	case ViewFeed:
		if v, ok := m.feed.SelectedVideo(); ok {
			return &v
		}
	case ViewDetail:
		return m.detail.Video()
	}
	return nil
}

func (m *Model) openComments() tea.Cmd {
	v := m.selectedVideo()
	if v == nil {
		return nil
	}
	m.switchTo(ViewComments)
	return m.comments.LoadComments(v.CommentsToken)
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
	m.statusSeq++
	m.statusMsg = msg
	m.resizeViews()
	seq := m.statusSeq
	return tea.Tick(clearAfter, func(time.Time) tea.Msg {
		return clearStatusMsg{seq: seq}
	})
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
	tabs := m.renderTabs()
	helpView := statusBarStyle.Render(m.help.View(m.keys))
	overhead := lipgloss.Height(tabs) + lipgloss.Height(helpView)
	if m.statusMsg != "" || m.downloading {
		overhead++
	}
	h := m.height - overhead
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) resizeViews() {
	if m.width == 0 {
		return
	}
	ch := m.contentHeight()
	m.search.SetSize(m.width, ch)
	m.detail.SetSize(m.width, ch)
	m.feed.SetSize(m.width, ch)
	m.subs.SetSize(m.width, ch)
	m.comments.SetSize(m.width, ch)
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
		return m.feed.View()
	case ViewSubs:
		return m.subs.View()
	case ViewComments:
		return m.comments.View()
	}
	return ""
}

func (m *Model) renderPlaceholder(label string) string {
	return placeholderStyle.
		Width(m.width).
		Height(m.contentHeight()).
		Render(label)
}
