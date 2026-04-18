package urlinput

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

var (
	overlayStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.Red).
			Padding(1, 2)

	errorStyle = lipgloss.NewStyle().Foreground(styles.Red)

	submitKey = key.NewBinding(key.WithKeys("enter"))
	cancelKey = key.NewBinding(key.WithKeys("esc"))
)

// SubmitMsg is emitted when the user submits a valid URL.
type SubmitMsg struct {
	Parsed youtube.ParsedURL
}

// CancelMsg is emitted when the user cancels the dialog.
type CancelMsg struct{}

// Model is a modal URL input dialog.
type Model struct {
	input  textinput.Model
	active bool
	errMsg string
	width  int
	height int
}

// New creates a new URL input model.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "Paste YouTube URL or video ID..."
	ti.CharLimit = 512
	return Model{input: ti}
}

// Show activates the dialog.
func (m *Model) Show(w, h int) tea.Cmd {
	m.active = true
	m.width = w
	m.height = h
	m.errMsg = ""
	m.input.SetValue("")
	m.input.Focus()
	return textinput.Blink
}

// IsActive reports whether the dialog is visible.
func (m Model) IsActive() bool {
	return m.active
}

// Update handles messages when the dialog is active.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, cancelKey):
			m.active = false
			m.input.Blur()
			return m, func() tea.Msg { return CancelMsg{} }

		case key.Matches(msg, submitKey):
			val := m.input.Value()
			if val == "" {
				return m, nil
			}
			parsed := youtube.ParseYouTubeURL(val)
			if parsed.Kind == youtube.URLUnknown {
				m.errMsg = "Unrecognized URL format"
				return m, nil
			}
			m.active = false
			m.input.Blur()
			m.errMsg = ""
			return m, func() tea.Msg { return SubmitMsg{Parsed: parsed} }
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// View renders the centered overlay dialog.
func (m Model) View() string {
	if !m.active {
		return ""
	}

	inputWidth := m.width*2/3 - 8
	if inputWidth > 80 {
		inputWidth = 80
	}
	if inputWidth < 30 {
		inputWidth = 30
	}
	m.input.SetWidth(inputWidth)

	title := styles.Title.Render("Open URL")
	input := m.input.View()

	var content string
	if m.errMsg != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, title, "", input, errorStyle.Render(m.errMsg))
	} else {
		content = lipgloss.JoinVertical(lipgloss.Left, title, "", input)
	}

	overlay := overlayStyle.Width(inputWidth + 4).Render(content)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		overlay,
	)
}
