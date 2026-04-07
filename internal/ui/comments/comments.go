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

	downKey     = key.NewBinding(key.WithKeys("j", "down"))
	upKey       = key.NewBinding(key.WithKeys("k", "up"))
	pageDownKey = key.NewBinding(key.WithKeys("ctrl+d", "pgdown"))
	pageUpKey   = key.NewBinding(key.WithKeys("ctrl+u", "pgup"))
	goTopKey    = key.NewBinding(key.WithKeys("g", "home"))
	goBottomKey = key.NewBinding(key.WithKeys("G", "end"))
	expandKey   = key.NewBinding(key.WithKeys("l"))
	collapseKey = key.NewBinding(key.WithKeys("h"))
	loadMoreKey = key.NewBinding(key.WithKeys("L"))
)

// LoadedMsg carries loaded comment results. Set the Source field
// to route the message to the correct Model instance.
type LoadedMsg struct {
	Source    string // opaque identifier to match against Model.source
	Comments []youtube.Comment
	NextToken string
	Err       error
}

// RepliesLoadedMsg carries loaded reply results.
type RepliesLoadedMsg struct {
	Source    string
	CommentID string
	Replies   []youtube.Comment
	NextToken string
	Err       error
}

// LoadFunc fetches a page of comments. The Model calls this for
// initial loads and "load more" pagination.
type LoadFunc func(ctx context.Context, token string) (*youtube.Page[youtube.Comment], error)

// ReplyFunc fetches replies for a comment thread.
type ReplyFunc func(ctx context.Context, commentID, token string) (*youtube.Page[youtube.Comment], error)

type thread struct {
	comment        youtube.Comment
	replies        []youtube.Comment
	expanded       bool
	loading        bool
	replyNextToken string
}

// Model manages a comment list with cursor navigation, expand/collapse,
// reply loading, and auto-pagination. It owns a viewport for rendering.
type Model struct {
	Viewport viewport.Model
	spinner  spinner.Model
	source   string // identifies this instance for message routing

	threads       []thread
	token         string
	loading       bool
	loaded        bool
	cursor        int
	cursorLine    int
	width         int
	height        int

	loadFn  LoadFunc
	replyFn ReplyFunc
}

// New creates a new comment model. source is an opaque string used to
// route LoadedMsg/RepliesLoadedMsg to the correct instance.
func New(source string, loadFn LoadFunc, replyFn ReplyFunc) Model {
	return Model{
		spinner: styles.NewSpinner(),
		source:  source,
		loadFn:  loadFn,
		replyFn: replyFn,
	}
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.Viewport.Width = w
	m.Viewport.Height = h
}

// Load fetches the initial page of comments with the given token.
func (m *Model) Load(initialToken string) tea.Cmd {
	m.threads = nil
	m.token = ""
	m.loading = true
	m.loaded = false
	m.cursor = 0
	m.cursorLine = 0

	if initialToken == "" || m.loadFn == nil {
		m.loading = false
		return nil
	}

	source := m.source
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		page, err := m.loadFn(context.Background(), initialToken)
		if err != nil {
			return LoadedMsg{Source: source, Err: err}
		}
		return LoadedMsg{
			Source:    source,
			Comments: page.Items,
			NextToken: page.NextToken,
		}
	})
}

func (m *Model) loadMore() tea.Cmd {
	if m.loading || m.token == "" || m.loadFn == nil {
		return nil
	}
	m.loading = true
	token := m.token
	source := m.source
	return func() tea.Msg {
		page, err := m.loadFn(context.Background(), token)
		if err != nil {
			return LoadedMsg{Source: source, Err: err}
		}
		return LoadedMsg{
			Source:    source,
			Comments: page.Items,
			NextToken: page.NextToken,
		}
	}
}

func (m *Model) autoLoad() tea.Cmd {
	total := m.totalItems()
	if total > 0 && m.cursor >= total-5 {
		return m.loadMore()
	}
	return nil
}

func (m *Model) totalItems() int {
	n := 0
	for _, t := range m.threads {
		n++
		if t.expanded {
			n += len(t.replies)
		}
	}
	return n
}

// Update handles messages. Only processes LoadedMsg/RepliesLoadedMsg
// whose Source matches this model's source.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case LoadedMsg:
		if msg.Source != m.source {
			return m, nil
		}
		m.loading = false
		m.loaded = true
		if msg.Err == nil {
			for _, c := range msg.Comments {
				m.threads = append(m.threads, thread{
					comment:        c,
					replyNextToken: c.ReplyToken,
				})
			}
			m.token = msg.NextToken
		}
		m.Viewport.SetContent(m.render())
		return m, nil

	case RepliesLoadedMsg:
		if msg.Source != m.source {
			return m, nil
		}
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
		m.Viewport.SetContent(m.render())
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, downKey):
			m.moveCursor(1)
			return m, m.autoLoad()
		case key.Matches(msg, upKey):
			m.moveCursor(-1)
			return m, nil
		case key.Matches(msg, pageDownKey):
			m.moveCursor(5)
			return m, m.autoLoad()
		case key.Matches(msg, pageUpKey):
			m.moveCursor(-5)
			return m, nil
		case key.Matches(msg, goTopKey):
			m.moveCursor(-m.totalItems())
			return m, nil
		case key.Matches(msg, goBottomKey):
			m.moveCursor(m.totalItems())
			return m, m.autoLoad()
		case key.Matches(msg, expandKey):
			return m, m.expandAtCursor()
		case key.Matches(msg, collapseKey):
			m.collapseAtCursor()
			return m, nil
		case key.Matches(msg, loadMoreKey):
			return m, m.loadMore()
		}

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// View returns the rendered comments viewport.
func (m Model) View() string {
	if m.loading && !m.loaded {
		return m.spinner.View() + " Loading comments..."
	}
	return m.Viewport.View()
}

func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	max := m.totalItems() - 1
	if max < 0 {
		max = 0
	}
	if m.cursor > max {
		m.cursor = max
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.Viewport.SetContent(m.render())
	m.scrollToCursor()
}

func (m *Model) scrollToCursor() {
	yOff := m.Viewport.YOffset
	vh := m.height
	if m.cursorLine < yOff {
		m.Viewport.SetYOffset(m.cursorLine)
	} else if m.cursorLine >= yOff+vh-2 {
		m.Viewport.SetYOffset(m.cursorLine - vh/3)
	}
}

func (m *Model) cursorThreadIndex() int {
	pos := 0
	for i, t := range m.threads {
		threadEnd := pos + 1
		if t.expanded {
			threadEnd += len(t.replies)
		}
		if m.cursor >= pos && m.cursor < threadEnd {
			return i
		}
		pos = threadEnd
	}
	return -1
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
		m.Viewport.SetContent(m.render())
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
		m.cursor = pos
		t.expanded = false
		m.Viewport.SetContent(m.render())
		m.scrollToCursor()
	}
}

func (m *Model) loadRepliesCmd(t *thread) tea.Cmd {
	if m.replyFn == nil {
		return nil
	}
	t.loading = true
	m.Viewport.SetContent(m.render())

	token := t.replyNextToken
	if token == "" {
		token = t.comment.ReplyToken
	}
	commentID := t.comment.ID
	source := m.source
	return func() tea.Msg {
		page, err := m.replyFn(context.Background(), commentID, token)
		if err != nil {
			return RepliesLoadedMsg{Source: source, CommentID: commentID, Err: err}
		}
		return RepliesLoadedMsg{
			Source:    source,
			CommentID: commentID,
			Replies:   page.Items,
			NextToken: page.NextToken,
		}
	}
}

func (m *Model) render() string {
	var b strings.Builder
	lineNum := 0
	countLines := func(s string) int { return strings.Count(s, "\n") }

	if m.loading && len(m.threads) == 0 {
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
		selected := pos == m.cursor
		if selected {
			m.cursorLine = lineNum
		}
		chunk := m.renderComment(t.comment, false, selected)
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
				replySelected := pos == m.cursor
				if replySelected {
					m.cursorLine = lineNum
				}
				chunk := m.renderComment(r, true, replySelected)
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

	if m.token != "" {
		b.WriteString("\n")
		b.WriteString(styles.Dim.Render("Press L to load more comments"))
	}

	b.WriteString("\n\n")
	b.WriteString(styles.Dim.Render("[j/k] navigate  [l] expand  [h] collapse  [L] load more"))

	return b.String()
}

func (m *Model) renderComment(c youtube.Comment, isReply bool, selected bool) string {
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
