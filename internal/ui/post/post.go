package post

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/ui/comments"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

const (
	tabContent  = 0
	tabComments = 1

	subTabBarHeight = 1
)

var subTabNames = []string{"Post", "Comments"}

type clearTransmitMsg struct{}

// Model is the post detail view with Post/Comments sub-tabs.
type Model struct {
	activeTab    int
	post         youtube.Post
	infoViewport viewport.Model
	comments     comments.Model
	spinner      spinner.Model
	client       youtube.Client
	imgR         *ytimage.Renderer
	thumbTransmit string
	thumbPlace    string
	thumbPending  bool
	width         int
	height        int
}

// New creates a new post detail view.
func New(client youtube.Client, imgR *ytimage.Renderer) Model {
	return Model{
		spinner: styles.NewSpinner(),
		client:  client,
		imgR:    imgR,
	}
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	vh := h - subTabBarHeight
	if vh < 1 {
		vh = 1
	}
	m.infoViewport.Width = w
	m.infoViewport.Height = vh
	m.comments.SetSize(w, vh)
}

// Load sets up the post and fetches comments.
func (m *Model) Load(p youtube.Post) tea.Cmd {
	m.post = p
	m.activeTab = tabContent
	m.thumbTransmit = ""
	m.thumbPlace = ""
	m.thumbPending = false

	vh := m.height - subTabBarHeight
	if vh < 1 {
		vh = 1
	}
	m.infoViewport = viewport.New(m.width, vh)

	var cmds []tea.Cmd

	// Fetch thumbnail if available
	if m.imgR != nil {
		url := shared.BestThumbnailURL(p.Thumbnails)
		if url != "" {
			m.thumbPending = true
			cmd := m.imgR.FetchCmd(url, 40, 10)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	// Set up comments model. On initial load, token equals detailParams
	// and we call GetPostComments with empty pageToken (initial browse).
	// On continuation, token is a real continuation token.
	detailParams := p.DetailParams
	client := m.client
	// Post comments and replies both use Browse (not Next like video comments),
	// so both loadFn and replyFn go through GetPostComments with continuation tokens.
	m.comments = comments.New("post-"+p.ID, func(ctx context.Context, token string) (*youtube.Page[youtube.Comment], error) {
		if token == detailParams {
			return client.GetPostComments(ctx, detailParams, "")
		}
		return client.GetPostComments(ctx, "", token)
	}, func(ctx context.Context, commentID, token string) (*youtube.Page[youtube.Comment], error) {
		return client.GetPostComments(ctx, "", token)
	})
	m.comments.SetSize(m.width, vh)

	if detailParams != "" {
		cmds = append(cmds, m.comments.Load(detailParams))
	}

	m.infoViewport.SetContent(m.renderPostContent())
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case comments.LoadedMsg, comments.RepliesLoadedMsg:
		var cmd tea.Cmd
		m.comments, cmd = m.comments.Update(msg)
		return m, cmd

	case ytimage.ThumbnailLoadedMsg:
		if m.imgR != nil && m.thumbPending && m.imgR.HandleLoaded(msg) {
			m.thumbPending = false
			if msg.Err == nil && msg.Placeholder != "" {
				m.thumbTransmit = msg.TransmitStr
				m.thumbPlace = msg.Placeholder
				cmds = append(cmds, scheduleClearTransmit())
			}
			m.infoViewport.SetContent(m.renderPostContent())
		}
		return m, tea.Batch(cmds...)

	case clearTransmitMsg:
		m.thumbTransmit = ""
		return m, nil

	case tea.KeyMsg:
		switch {
		case msg.String() == "tab":
			m.activeTab = (m.activeTab + 1) % len(subTabNames)
			return m, nil
		case msg.String() == "shift+tab":
			m.activeTab = (m.activeTab - 1 + len(subTabNames)) % len(subTabNames)
			return m, nil
		}

		if m.activeTab == tabContent {
			var cmd tea.Cmd
			m.infoViewport, cmd = m.infoViewport.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			var cmd tea.Cmd
			m.comments, cmd = m.comments.Update(msg)
			return m, cmd
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.comments, cmd = m.comments.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	subBar := shared.RenderSubTabBar(subTabNames, m.activeTab)
	var content string
	if m.activeTab == tabContent {
		content = m.infoViewport.View()
	} else {
		content = m.comments.View()
	}

	view := lipgloss.JoinVertical(lipgloss.Left, subBar, content)
	if m.thumbTransmit != "" {
		view = m.thumbTransmit + view
	}
	return view
}

func scheduleClearTransmit() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return clearTransmitMsg{}
	})
}

func (m *Model) renderPostContent() string {
	p := m.post
	var b strings.Builder

	// Thumbnail
	if m.thumbPlace != "" {
		b.WriteString(m.thumbPlace)
		b.WriteString("\n\n")
	}

	// Author and time
	author := styles.Accent.Bold(true).Render(p.AuthorName)
	if p.PublishedAt != "" {
		author += styles.Dim.Render("  " + p.PublishedAt)
	}
	b.WriteString(author)
	b.WriteString("\n\n")

	// Content — wrapped to viewport width
	contentWidth := m.width - 2
	if contentWidth < 20 {
		contentWidth = 20
	}
	b.WriteString(styles.Subtitle.Width(contentWidth).Render(p.Content))
	b.WriteString("\n\n")

	// Likes
	if p.LikeCount != "" {
		b.WriteString(styles.Dim.Render(p.LikeCount + " likes"))
	}

	return b.String()
}
