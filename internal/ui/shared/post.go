package shared

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

// PostItem wraps a Post for use with bubbles/list.
type PostItem struct {
	Post youtube.Post
}

func (p PostItem) FilterValue() string { return p.Post.Content }
func (p PostItem) Title() string       { return p.Post.Content }
func (p PostItem) Description() string { return p.Post.AuthorName }

// PostDelegate renders post items in a text-only three-line list.
type PostDelegate struct{}

func (d PostDelegate) Height() int                             { return 3 }
func (d PostDelegate) Spacing() int                            { return 1 }
func (d PostDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d PostDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	pi, ok := item.(PostItem)
	if !ok {
		return
	}
	isSelected := index == m.Index()
	cursor := "  "
	if isSelected {
		cursor = "> "
	}
	titleStyle := styles.Title
	if isSelected {
		titleStyle = styles.SelectedTitle
	}

	p := pi.Post
	// Collapse newlines so multi-line post content fits in a single line
	oneLine := strings.Join(strings.Fields(p.Content), " ")
	title := titleStyle.Render(Truncate(oneLine, m.Width()-4))
	var meta string
	if p.LikeCount != "" {
		meta = styles.Dim.Render(p.LikeCount + " likes")
	}
	if p.PublishedAt != "" {
		if meta != "" {
			meta += styles.Dim.Render("  ")
		}
		meta += styles.Dim.Render(p.PublishedAt)
	}
	author := styles.Subtitle.Render(p.AuthorName)

	fmt.Fprintf(w, "%s%s\n  %s\n  %s", cursor, title, author, meta)
}
