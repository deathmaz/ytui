package comments

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

var (
	authorStyle  = styles.Accent.Bold(true)
	ownerStyle   = styles.SelectedTitle.Bold(true)
	timeStyle    = styles.Dim
	contentStyle = styles.Subtitle
	likesStyle   = styles.Dim
	replyIndent  = "    "

	selectKey   = key.NewBinding(key.WithKeys("enter"))
	expandKey   = key.NewBinding(key.WithKeys("l"))
	collapseKey = key.NewBinding(key.WithKeys("h"))
	loadMoreKey = key.NewBinding(key.WithKeys("L"))
	upKey       = key.NewBinding(key.WithKeys("k", "up"))
	downKey     = key.NewBinding(key.WithKeys("j", "down"))
	pageDownKey = key.NewBinding(key.WithKeys("ctrl+d", "ctrl+f", "pgdown"))
	pageUpKey   = key.NewBinding(key.WithKeys("ctrl+u", "ctrl+b", "pgup"))
)

type commentThread struct {
	comment        youtube.Comment
	replies        []youtube.Comment
	expanded       bool
	loading        bool
	replyNextToken string
}

// CommentsLoadedMsg carries comment results.
type CommentsLoadedMsg struct {
	Comments  []youtube.Comment
	NextToken string
	Err       error
}

// RepliesLoadedMsg carries reply results for a specific comment.
type RepliesLoadedMsg struct {
	CommentID string
	Replies   []youtube.Comment
	NextToken string
	Err       error
}

// Model is the comments view.
type Model struct {
	viewport   viewport.Model
	spinner    spinner.Model
	threads    []commentThread
	cursor     int
	cursorLine int
	loading   bool
	nextToken string
	width     int
	height    int
	client    youtube.Client
}

// New creates a new comments view model.
func New(client youtube.Client) Model {
	return Model{
		spinner: styles.NewSpinner(),
		client:  client,
	}
}

// SetSize updates the view dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.viewport.Width = w
	m.viewport.Height = h
}

// LoadComments starts loading comments for a video.
// commentsToken is the pre-extracted token from GetVideo (avoids an extra HTTP call).
func (m *Model) LoadComments(commentsToken string) tea.Cmd {
	m.loading = true
	m.threads = nil
	m.cursor = 0
	m.nextToken = ""
	m.viewport = viewport.New(m.width, m.height)
	m.viewport.KeyMap = scrollOnlyKeyMap()
	if commentsToken == "" {
		m.loading = false
		m.viewport.SetContent(styles.Dim.Render("No comments available"))
		return nil
	}
	client := m.client
	token := commentsToken
	return tea.Batch(m.spinner.Tick, func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = CommentsLoadedMsg{Err: fmt.Errorf("panic: %v", r)}
			}
		}()
		page, err := client.GetComments(context.Background(), "", token)
		if err != nil {
			return CommentsLoadedMsg{Err: err}
		}
		return CommentsLoadedMsg{
			Comments:  page.Items,
			NextToken: page.NextToken,
		}
	})
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case CommentsLoadedMsg:
		m.loading = false
		if msg.Err != nil {
			m.viewport = viewport.New(m.width, m.height)
			m.viewport.KeyMap = scrollOnlyKeyMap()
			m.viewport.SetContent(styles.Accent.Render("Error: " + msg.Err.Error()))
			return m, nil
		}
		for _, c := range msg.Comments {
			m.threads = append(m.threads, commentThread{comment: c})
		}
		m.nextToken = msg.NextToken
		m.rebuildViewport()
		return m, nil

	case RepliesLoadedMsg:
		for i := range m.threads {
			if m.threads[i].comment.ID == msg.CommentID {
				m.threads[i].loading = false
				if msg.Err != nil {
					m.threads[i].replyNextToken = ""
					break
				}
				m.threads[i].replies = append(m.threads[i].replies, msg.Replies...)
				m.threads[i].replyNextToken = msg.NextToken
				m.threads[i].expanded = true
				break
			}
		}
		m.rebuildViewport()
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			break
		}

		switch {
		case key.Matches(msg, downKey):
			if m.cursor < m.totalItems()-1 {
				m.cursor++
				m.rebuildViewport()
				m.scrollToCursor()
			}
			return m, nil

		case key.Matches(msg, upKey):
			if m.cursor > 0 {
				m.cursor--
				m.rebuildViewport()
				m.scrollToCursor()
			}
			return m, nil

		case key.Matches(msg, pageDownKey):
			m.moveCursor(m.height / 2)
			return m, nil

		case key.Matches(msg, pageUpKey):
			m.moveCursor(-m.height / 2)
			return m, nil

		case key.Matches(msg, selectKey), key.Matches(msg, expandKey):
			return m, m.expandReplies()

		case key.Matches(msg, collapseKey):
			m.collapseCurrentThread()
			return m, nil

		case key.Matches(msg, loadMoreKey):
			return m, m.loadMore()
		}

	case spinner.TickMsg:
		if m.loading || m.hasLoadingThread() {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.loading && len(m.threads) == 0 {
		return m.spinner.View() + " Loading comments..."
	}
	return m.viewport.View()
}

func (m *Model) expandReplies() tea.Cmd {
	threadIdx, _ := m.cursorPosition()
	if threadIdx < 0 || threadIdx >= len(m.threads) {
		return nil
	}

	t := &m.threads[threadIdx]

	if t.comment.ReplyCount == 0 && t.comment.ReplyToken == "" {
		return nil
	}

	// Already expanded -- load more replies if available
	if t.expanded {
		if t.replyNextToken == "" {
			return nil
		}
		return m.loadRepliesCmd(t)
	}

	// Already loaded before, just re-expand
	if len(t.replies) > 0 {
		t.expanded = true
		m.rebuildViewport()
		return nil
	}

	// Need to load from scratch
	if t.comment.ReplyToken == "" {
		return nil
	}
	return m.loadRepliesCmd(t)
}

func (m *Model) loadRepliesCmd(t *commentThread) tea.Cmd {
	t.loading = true
	m.rebuildViewport()

	token := t.replyNextToken
	if token == "" {
		token = t.comment.ReplyToken
	}
	commentID := t.comment.ID
	client := m.client
	return tea.Batch(m.spinner.Tick, func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = RepliesLoadedMsg{CommentID: commentID, Err: fmt.Errorf("panic: %v", r)}
			}
		}()
		page, err := client.GetReplies(context.Background(), commentID, token)
		if err != nil {
			return RepliesLoadedMsg{CommentID: commentID, Err: err}
		}
		return RepliesLoadedMsg{
			CommentID: commentID,
			Replies:   page.Items,
			NextToken: page.NextToken,
		}
	})
}

func (m *Model) collapseCurrentThread() {
	threadIdx, _ := m.cursorPosition()
	if threadIdx < 0 || threadIdx >= len(m.threads) {
		return
	}
	t := &m.threads[threadIdx]
	if !t.expanded {
		return
	}

	// Move cursor to this thread's top-level comment
	pos := 0
	for i := 0; i < threadIdx; i++ {
		pos++
		if m.threads[i].expanded {
			pos += len(m.threads[i].replies)
		}
	}
	// pos is now the position of thread[threadIdx]'s comment
	m.cursor = pos

	t.expanded = false
	m.rebuildViewport()
	m.scrollToCursor()
}

func (m *Model) loadMore() tea.Cmd {
	if m.nextToken == "" || m.loading {
		return nil
	}
	m.loading = true
	token := m.nextToken
	client := m.client
	return tea.Batch(m.spinner.Tick, func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = CommentsLoadedMsg{Err: fmt.Errorf("panic: %v", r)}
			}
		}()
		page, err := client.GetComments(context.Background(), "", token)
		if err != nil {
			return CommentsLoadedMsg{Err: err}
		}
		return CommentsLoadedMsg{
			Comments:  page.Items,
			NextToken: page.NextToken,
		}
	})
}

func (m Model) totalItems() int {
	n := 0
	for _, t := range m.threads {
		n++
		if t.expanded {
			n += len(t.replies)
		}
	}
	return n
}

// cursorPosition returns which thread and reply index the cursor is on.
// replyIdx = -1 means the cursor is on the top-level comment.
func (m Model) cursorPosition() (threadIdx, replyIdx int) {
	pos := 0
	for i, t := range m.threads {
		if pos == m.cursor {
			return i, -1
		}
		pos++
		if t.expanded {
			for j := range t.replies {
				if pos == m.cursor {
					return i, j
				}
				pos++
			}
		}
	}
	return -1, -1
}

func (m *Model) rebuildViewport() {
	content, cursorLine := m.renderCommentsWithCursorLine()
	if m.viewport.Width == 0 {
		m.viewport = viewport.New(m.width, m.height)
	}
	m.viewport.KeyMap = scrollOnlyKeyMap()
	m.viewport.SetContent(content)
	m.cursorLine = cursorLine
}

func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	max := m.totalItems() - 1
	if m.cursor > max {
		m.cursor = max
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.rebuildViewport()
	m.scrollToCursor()
}

func (m *Model) scrollToCursor() {
	// Keep cursor visible, roughly in the top third
	yOff := m.viewport.YOffset
	if m.cursorLine < yOff {
		// Cursor above viewport
		m.viewport.SetYOffset(m.cursorLine)
	} else if m.cursorLine >= yOff+m.height-2 {
		// Cursor below viewport
		m.viewport.SetYOffset(m.cursorLine - m.height/3)
	}
}

func (m Model) hasLoadingThread() bool {
	for _, t := range m.threads {
		if t.loading {
			return true
		}
	}
	return false
}

func (m Model) renderCommentsWithCursorLine() (string, int) {
	if len(m.threads) == 0 {
		return styles.Dim.Render("No comments"), 0
	}

	curThread, curReplyIdx := m.cursorPosition()

	var b strings.Builder
	lineNum := 0
	cursorLine := 0
	pos := 0

	countLines := func(s string) int {
		return strings.Count(s, "\n")
	}

	for i, t := range m.threads {
		if i > 0 {
			b.WriteString("\n")
			lineNum++
		}
		selected := pos == m.cursor
		if selected {
			cursorLine = lineNum
		}
		chunk := m.renderCommentStr(t.comment, selected, false)
		b.WriteString(chunk)
		lineNum += countLines(chunk)
		pos++

		if t.comment.ReplyCount > 0 && !t.expanded && !t.loading {
			indicator := fmt.Sprintf("  ▸ %d replies [l/enter to expand]", t.comment.ReplyCount)
			if selected {
				b.WriteString(styles.Accent.Render(indicator))
			} else {
				b.WriteString(styles.Dim.Render(indicator))
			}
			b.WriteString("\n")
			lineNum++
		}

		if t.loading {
			line := "  " + m.spinner.View() + " Loading replies...\n"
			b.WriteString(line)
			lineNum++
		}

		if t.expanded {
			for j, r := range t.replies {
				replySelected := curThread == i && curReplyIdx == j
				if replySelected {
					cursorLine = lineNum
				}
				chunk := m.renderCommentStr(r, replySelected, true)
				b.WriteString(chunk)
				lineNum += countLines(chunk)
				pos++
			}
			if t.replyNextToken != "" {
				b.WriteString(replyIndent)
				b.WriteString(styles.Accent.Render("  ▸ more replies [l/enter]"))
				b.WriteString("\n")
				lineNum++
			}
			b.WriteString(replyIndent)
			b.WriteString(styles.Dim.Render("  ▾ [h] collapse"))
			b.WriteString("\n")
			lineNum++
		}
	}

	if m.nextToken != "" {
		b.WriteString("\n")
		b.WriteString(styles.Dim.Render("Press L to load more comments"))
	}

	b.WriteString("\n\n")
	b.WriteString(styles.Dim.Render("[esc] back  [j/k] navigate  [l] expand  [h] collapse  [L] more comments"))

	return b.String(), cursorLine
}

func (m Model) renderCommentStr(c youtube.Comment, selected, isReply bool) string {
	var b strings.Builder
	indent := ""
	if isReply {
		indent = replyIndent
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

func scrollOnlyKeyMap() viewport.KeyMap {
	return viewport.KeyMap{}
}
