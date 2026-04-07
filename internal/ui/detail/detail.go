package detail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

var (
	separatorStyle = styles.Dim
	authorStyle    = styles.Accent.Bold(true)
	ownerStyle     = styles.SelectedTitle.Bold(true)
	timeStyle      = styles.Dim
	contentStyle   = styles.Subtitle
	likesStyle     = styles.Dim

	loadMoreKey = key.NewBinding(key.WithKeys("L"))
	expandKey   = key.NewBinding(key.WithKeys("l"))
	collapseKey = key.NewBinding(key.WithKeys("h"))
	upKey       = key.NewBinding(key.WithKeys("k", "up"))
	downKey     = key.NewBinding(key.WithKeys("j", "down"))
	pageDownKey = key.NewBinding(key.WithKeys("ctrl+d", "ctrl+f", "pgdown"))
	pageUpKey   = key.NewBinding(key.WithKeys("ctrl+u", "ctrl+b", "pgup"))
	goTopKey    = key.NewBinding(key.WithKeys("g", "home"))
	goBottomKey = key.NewBinding(key.WithKeys("G", "end"))
)

const (
	thumbCols = 40
	thumbRows = 10

	tabInfo     = 0
	tabComments = 1
)

// VideoLoadedMsg carries the loaded video details.
type VideoLoadedMsg struct {
	Video *youtube.Video
	Err   error
}

type clearTransmitMsg struct{}

type commentsLoadedMsg struct {
	Comments  []youtube.Comment
	NextToken string
	Err       error
}

type repliesLoadedMsg struct {
	CommentID string
	Replies   []youtube.Comment
	NextToken string
	Err       error
}

type commentThread struct {
	comment        youtube.Comment
	replies        []youtube.Comment
	expanded       bool
	loading        bool
	replyNextToken string
}

// Model is the video detail view with Info and Comments sub-tabs.
type Model struct {
	activeTab        int // 0=Info, 1=Comments
	infoViewport     viewport.Model
	commentsViewport viewport.Model
	spinner          spinner.Model
	video            *youtube.Video
	loading          bool
	width            int
	height           int
	client           youtube.Client
	imgR             *ytimage.Renderer
	thumbTransmit    string
	thumbPlace       string
	thumbPending     bool
	thumbFailed      bool

	// Comments
	threads         []commentThread
	commentsToken   string
	commentsLoading bool
	commentCursor   int // index into threads (+ expanded replies)
	cursorLine      int // line number of cursor in rendered comments
}

// New creates a new detail view model.
func New(client youtube.Client, imgR *ytimage.Renderer) Model {
	return Model{
		spinner: styles.NewSpinner(),
		client:  client,
		imgR:    imgR,
	}
}

const subTabBarHeight = 1

func (m *Model) viewportHeight() int {
	h := m.height - subTabBarHeight
	if h < 1 {
		h = 1
	}
	return h
}

// SetSize updates the view dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	vh := m.viewportHeight()
	m.infoViewport.Width = w
	m.infoViewport.Height = vh
	m.commentsViewport.Width = w
	m.commentsViewport.Height = vh
	if m.video != nil {
		if m.activeTab == tabInfo {
			m.infoViewport.SetContent(m.renderInfo())
		} else {
			m.commentsViewport.SetContent(m.renderComments())
		}
	}
}

// LoadVideo starts loading a video's details.
func (m *Model) LoadVideo(id string) tea.Cmd {
	m.loading = true
	m.activeTab = tabInfo
	m.video = nil
	m.thumbTransmit = ""
	m.thumbPlace = ""
	m.thumbPending = false
	m.thumbFailed = false
	m.threads = nil
	m.commentsToken = ""
	m.commentsLoading = false
	m.commentCursor = 0
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
		vh := m.viewportHeight()
		if msg.Err != nil {
			m.infoViewport = viewport.New(m.width, vh)
			m.infoViewport.KeyMap = viewportKeyMap()
			m.infoViewport.SetContent(fmt.Sprintf("Error loading video: %v", msg.Err))
			return m, nil
		}
		m.video = msg.Video
		m.infoViewport = viewport.New(m.width, vh)
		m.infoViewport.KeyMap = viewportKeyMap()
		m.commentsViewport = viewport.New(m.width, vh)
		m.commentsViewport.KeyMap = viewportKeyMap()

		// Start thumbnail fetch
		if m.imgR != nil {
			thumbURL := shared.BestThumbnail(*m.video)
			if thumbURL != "" {
				tx, pl := m.imgR.Get(thumbURL)
				if pl != "" {
					m.thumbTransmit = tx
					m.thumbPlace = pl
					cmds = append(cmds, scheduleClearTransmit())
				} else {
					m.thumbPending = true
					cmds = append(cmds, m.imgR.FetchCmd(thumbURL, thumbCols, thumbRows))
				}
			}
		}

		// Auto-fetch comments
		if m.video.CommentsToken != "" {
			m.commentsToken = m.video.CommentsToken
			m.commentsLoading = true
			token := m.commentsToken
			client := m.client
			cmds = append(cmds, func() tea.Msg {
				page, err := client.GetComments(context.Background(), "", token)
				if err != nil {
					return commentsLoadedMsg{Err: err}
				}
				return commentsLoadedMsg{Comments: page.Items, NextToken: page.NextToken}
			})
		}

		m.infoViewport.SetContent(m.renderInfo())
		return m, tea.Batch(cmds...)

	case ytimage.ThumbnailLoadedMsg:
		// Only process if this fetch was initiated by our renderer
		// (not by a list thumbnail renderer sharing the message bus).
		if m.imgR != nil && m.imgR.HandleLoaded(msg) {
			m.thumbPending = false
			if msg.Err == nil && msg.Placeholder != "" {
				m.thumbTransmit = msg.TransmitStr
				m.thumbPlace = msg.Placeholder
				cmds = append(cmds, scheduleClearTransmit())
			} else {
				m.thumbFailed = true
			}
			if m.video != nil {
				m.infoViewport.SetContent(m.renderInfo())
			}
		}
		return m, tea.Batch(cmds...)

	case clearTransmitMsg:
		m.thumbTransmit = ""
		return m, nil

	case commentsLoadedMsg:
		m.commentsLoading = false
		if msg.Err == nil {
			for _, c := range msg.Comments {
				m.threads = append(m.threads, commentThread{comment: c})
			}
			m.commentsToken = msg.NextToken
		}
		if m.video != nil {
			m.commentsViewport.SetContent(m.renderComments())
		}
		return m, nil

	case repliesLoadedMsg:
		for i := range m.threads {
			if m.threads[i].comment.ID == msg.CommentID {
				m.threads[i].loading = false
				if msg.Err == nil {
					m.threads[i].replies = append(m.threads[i].replies, msg.Replies...)
					m.threads[i].replyNextToken = msg.NextToken
					m.threads[i].expanded = true
				}
				break
			}
		}
		if m.video != nil {
			m.commentsViewport.SetContent(m.renderComments())
		}
		return m, nil

	case tea.KeyMsg:
		if !m.loading {
			// Tab switching
			if msg.String() == "tab" {
				if m.activeTab == tabInfo {
					m.activeTab = tabComments
				}
				return m, nil
			}
			if msg.String() == "shift+tab" {
				if m.activeTab == tabComments {
					m.activeTab = tabInfo
				}
				return m, nil
			}

			if m.activeTab == tabInfo {
				switch {
				case key.Matches(msg, goTopKey):
					m.infoViewport.GotoTop()
					return m, nil
				case key.Matches(msg, goBottomKey):
					m.infoViewport.GotoBottom()
					return m, nil
				default:
					var cmd tea.Cmd
					m.infoViewport, cmd = m.infoViewport.Update(msg)
					cmds = append(cmds, cmd)
				}
			} else {
				// Comments tab — cursor navigation always active
				switch {
				case key.Matches(msg, downKey):
					m.moveCommentCursor(1)
					return m, m.autoLoadComments()
				case key.Matches(msg, upKey):
					m.moveCommentCursor(-1)
					return m, nil
				case key.Matches(msg, pageDownKey):
					m.moveCommentCursor(5)
					return m, m.autoLoadComments()
				case key.Matches(msg, pageUpKey):
					m.moveCommentCursor(-5)
					return m, nil
				case key.Matches(msg, goTopKey):
					m.moveCommentCursor(-m.totalCommentItems())
					return m, nil
				case key.Matches(msg, goBottomKey):
					m.moveCommentCursor(m.totalCommentItems())
					return m, m.autoLoadComments()
				case key.Matches(msg, expandKey):
					return m, m.expandAtCursor()
				case key.Matches(msg, collapseKey):
					m.collapseAtCursor()
					return m, nil
				case key.Matches(msg, loadMoreKey):
					return m, m.loadMoreComments()
				}
			}
		}

	case spinner.TickMsg:
		if m.loading || m.commentsLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func scheduleClearTransmit() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return clearTransmitMsg{}
	})
}

var subTabNames = []string{"Info", "Comments"}

func (m Model) renderSubTabBar() string {
	return shared.RenderSubTabBar(subTabNames, m.activeTab)
}

func (m Model) View() string {
	if m.loading {
		return m.spinner.View() + " Loading video details..."
	}

	subBar := m.renderSubTabBar()
	var content string
	if m.activeTab == tabInfo {
		content = m.infoViewport.View()
	} else {
		content = m.commentsViewport.View()
	}

	view := lipgloss.JoinVertical(lipgloss.Left, subBar, content)
	if m.thumbTransmit != "" {
		view = m.thumbTransmit + view
	}
	return view
}

func (m *Model) loadMoreComments() tea.Cmd {
	if m.commentsLoading || m.commentsToken == "" {
		return nil
	}
	m.commentsLoading = true
	token := m.commentsToken
	client := m.client
	return func() tea.Msg {
		page, err := client.GetComments(context.Background(), "", token)
		if err != nil {
			return commentsLoadedMsg{Err: err}
		}
		return commentsLoadedMsg{Comments: page.Items, NextToken: page.NextToken}
	}
}

func (m *Model) cursorThreadIndex() int {
	pos := 0
	for i, t := range m.threads {
		threadEnd := pos + 1
		if t.expanded {
			threadEnd += len(t.replies)
		}
		if m.commentCursor >= pos && m.commentCursor < threadEnd {
			return i
		}
		pos = threadEnd
	}
	return -1
}

func (m *Model) autoLoadComments() tea.Cmd {
	total := m.totalCommentItems()
	if total > 0 && m.commentCursor >= total-5 {
		return m.loadMoreComments()
	}
	return nil
}

func (m *Model) totalCommentItems() int {
	n := 0
	for _, t := range m.threads {
		n++
		if t.expanded {
			n += len(t.replies)
		}
	}
	return n
}

func (m *Model) moveCommentCursor(delta int) {
	m.commentCursor += delta
	max := m.totalCommentItems() - 1
	if max < 0 {
		max = 0
	}
	if m.commentCursor > max {
		m.commentCursor = max
	}
	if m.commentCursor < 0 {
		m.commentCursor = 0
	}
	m.commentsViewport.SetContent(m.renderComments())
	m.scrollToCommentCursor()
}

func (m *Model) expandAtCursor() tea.Cmd {
	idx := m.cursorThreadIndex()
	if idx < 0 || idx >= len(m.threads) {
		return nil
	}
	t := &m.threads[idx]

	if t.comment.ReplyCount == 0 && t.comment.ReplyToken == "" {
		return nil
	}
	if t.expanded {
		if t.replyNextToken == "" {
			return nil
		}
		return m.loadRepliesCmd(t)
	}
	if len(t.replies) > 0 {
		t.expanded = true
		m.commentsViewport.SetContent(m.renderComments())
		return nil
	}
	if t.comment.ReplyToken == "" {
		return nil
	}
	return m.loadRepliesCmd(t)
}

func (m *Model) collapseAtCursor() {
	idx := m.cursorThreadIndex()
	if idx < 0 || idx >= len(m.threads) {
		return
	}
	t := &m.threads[idx]
	if t.expanded {
		pos := 0
		for i := 0; i < idx; i++ {
			pos++
			if m.threads[i].expanded {
				pos += len(m.threads[i].replies)
			}
		}
		m.commentCursor = pos
		t.expanded = false
		m.commentsViewport.SetContent(m.renderComments())
		m.scrollToCommentCursor()
	}
}

func (m *Model) loadRepliesCmd(t *commentThread) tea.Cmd {
	t.loading = true
	m.commentsViewport.SetContent(m.renderComments())

	token := t.replyNextToken
	if token == "" {
		token = t.comment.ReplyToken
	}
	commentID := t.comment.ID
	client := m.client
	return func() tea.Msg {
		page, err := client.GetReplies(context.Background(), commentID, token)
		if err != nil {
			return repliesLoadedMsg{CommentID: commentID, Err: err}
		}
		return repliesLoadedMsg{
			CommentID: commentID,
			Replies:   page.Items,
			NextToken: page.NextToken,
		}
	}
}

func (m *Model) scrollToCommentCursor() {
	yOff := m.commentsViewport.YOffset
	vh := m.viewportHeight()
	if m.cursorLine < yOff {
		m.commentsViewport.SetYOffset(m.cursorLine)
	} else if m.cursorLine >= yOff+vh-2 {
		m.commentsViewport.SetYOffset(m.cursorLine - vh/3)
	}
}

// renderInfo renders the Info tab content.
func (m *Model) renderInfo() string {
	v := m.video
	if v == nil {
		return ""
	}

	var b strings.Builder
	sep := separatorStyle.Render(strings.Repeat("─", m.width-2))

	// Thumbnail
	if m.thumbPlace != "" {
		b.WriteString(m.thumbPlace)
		b.WriteString("\n\n")
	} else if m.thumbPending {
		for i := 0; i < thumbRows+1; i++ {
			b.WriteString("\n")
		}
	}

	// Title
	b.WriteString(styles.Title.MarginBottom(1).Width(m.width - 2).Render(v.Title))
	b.WriteString("\n")

	// Channel
	channel := styles.Accent.Bold(true).Render(v.ChannelName)
	if v.SubscriberCount != "" {
		channel += styles.Subtitle.Render("  " + v.SubscriberCount)
	}
	b.WriteString(channel)
	b.WriteString("\n\n")

	// Stats
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

	// URL
	b.WriteString(styles.Subtitle.Render(v.URL))
	b.WriteString("\n\n")
	b.WriteString(sep)
	b.WriteString("\n\n")

	// Description
	if v.Description != "" {
		b.WriteString(contentStyle.Width(m.width - 2).Render(v.Description))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Dim.Render("[p] play  [d] download  [o] open in browser  [y] copy URL  [tab] comments  [esc] back"))

	return b.String()
}

// renderComments renders the Comments tab content.
func (m *Model) renderComments() string {
	var b strings.Builder
	lineNum := 0
	countLines := func(s string) int { return strings.Count(s, "\n") }

	if m.commentsLoading && len(m.threads) == 0 {
		b.WriteString(m.spinner.View() + " Loading comments...")
		return b.String()
	}
	if len(m.threads) == 0 {
		b.WriteString(styles.Dim.Render("No comments"))
		return b.String()
	}

	pos := 0
	for i, t := range m.threads {
		if i > 0 {
			b.WriteString("\n")
			lineNum++
		}
		selected := pos == m.commentCursor
		if selected {
			m.cursorLine = lineNum
		}
		chunk := m.renderCommentStr(t.comment, false, selected)
		b.WriteString(chunk)
		lineNum += countLines(chunk)
		pos++

		if t.comment.ReplyCount > 0 && !t.expanded {
			if t.loading {
				b.WriteString("  " + m.spinner.View() + " Loading replies...\n")
			} else {
				indicator := fmt.Sprintf("  ▸ %d replies [l to expand]", t.comment.ReplyCount)
				if selected {
					b.WriteString(styles.Accent.Render(indicator))
				} else {
					b.WriteString(styles.Dim.Render(indicator))
				}
				b.WriteString("\n")
			}
			lineNum++
		}

		if t.expanded {
			for _, r := range t.replies {
				replySelected := pos == m.commentCursor
				if replySelected {
					m.cursorLine = lineNum
				}
				chunk := m.renderCommentStr(r, true, replySelected)
				b.WriteString(chunk)
				lineNum += countLines(chunk)
				pos++
			}
			if t.loading {
				b.WriteString("      " + m.spinner.View() + " Loading replies...\n")
				lineNum++
			}
			if t.replyNextToken != "" {
				b.WriteString("      ")
				b.WriteString(styles.Accent.Render("▸ more replies [l]"))
				b.WriteString("\n")
				lineNum++
			}
			b.WriteString("      ")
			b.WriteString(styles.Dim.Render("▾ [h] collapse"))
			b.WriteString("\n")
			lineNum++
		}
	}

	if m.commentsToken != "" {
		b.WriteString("\n")
		b.WriteString(styles.Dim.Render("Press L to load more comments"))
	}

	b.WriteString("\n\n")
	b.WriteString(styles.Dim.Render("[j/k] navigate  [l] expand  [h] collapse  [L] load more  [shift+tab] info  [esc] back"))

	return b.String()
}

func (m *Model) renderCommentStr(c youtube.Comment, isReply bool, selected bool) string {
	var b strings.Builder
	indent := ""
	if isReply {
		indent = "    "
	}

	cursor := "  "
	if selected {
		cursor = "> "
	}

	aStyle := authorStyle
	if c.IsOwner {
		aStyle = ownerStyle
	}
	b.WriteString(indent)
	b.WriteString(cursor)
	b.WriteString(aStyle.Render(c.AuthorName))
	b.WriteString("  ")
	b.WriteString(timeStyle.Render(c.PublishedAt))
	b.WriteString("\n")

	contentWidth := m.width - len(indent) - 4
	if contentWidth < 20 {
		contentWidth = 20
	}
	content := contentStyle.Width(contentWidth).Render(c.Content)
	for _, line := range strings.Split(content, "\n") {
		b.WriteString(indent)
		b.WriteString("    ")
		b.WriteString(line)
		b.WriteString("\n")
	}

	likes := strings.TrimSpace(c.LikeCount)
	if likes == "" {
		likes = "0"
	}
	b.WriteString(indent)
	b.WriteString("    ")
	b.WriteString(likesStyle.Render("👍 " + likes))
	b.WriteString("\n")
	return b.String()
}


func viewportKeyMap() viewport.KeyMap {
	return viewport.KeyMap{
		PageDown:     key.NewBinding(key.WithKeys("ctrl+f", "pgdown")),
		PageUp:       key.NewBinding(key.WithKeys("ctrl+b", "pgup")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u")),
		Up:           key.NewBinding(key.WithKeys("k", "up")),
		Down:         key.NewBinding(key.WithKeys("j", "down")),
	}
}
