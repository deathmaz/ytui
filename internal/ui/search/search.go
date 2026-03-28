package search

import (
	"context"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

var inputStyle = lipgloss.NewStyle().Padding(0, 1)

type focus int

const (
	focusInput focus = iota
	focusList
)

// SearchResultMsg carries search results back to the model.
type SearchResultMsg struct {
	Videos    []youtube.Video
	NextToken string
	Append    bool // true when loading more results
	Err       error
}

// VideoSelectedMsg is emitted when a user selects a video.
type VideoSelectedMsg struct {
	Video youtube.Video
}

// Model is the search view model.
type Model struct {
	input      textinput.Model
	results    list.Model
	spinner    spinner.Model
	keys       keyMap
	focused    focus
	searching  bool
	query      string
	nextToken  string
	loadingMore bool
	width      int
	height     int
	client     youtube.Client
}

// New creates a new search view model.
func New(client youtube.Client) Model {
	ti := textinput.New()
	ti.Placeholder = "Search YouTube..."
	ti.CharLimit = 256
	ti.Focus()

	delegate := videoDelegate{}
	l := list.New(nil, delegate, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.SetShowPagination(true)
	l.KeyMap.Quit = key.NewBinding() // disable list's built-in quit

	return Model{
		input:   ti,
		results: l,
		spinner: styles.NewSpinner(),
		keys:    defaultKeyMap(),
		focused: focusInput,
		client:  client,
	}
}

// SetSize updates the view dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	inputHeight := 3 // input + padding
	m.results.SetSize(w, h-inputHeight)
	m.input.Width = w - 4
}

// Focus gives focus to the search input.
func (m *Model) Focus() {
	m.focused = focusInput
	m.input.Focus()
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.FocusInput) && m.focused == focusList:
			m.focused = focusInput
			m.input.Focus()
			return m, textinput.Blink

		case key.Matches(msg, m.keys.Submit) && m.focused == focusInput:
			query := m.input.Value()
			if query == "" {
				return m, nil
			}
			m.query = query
			m.nextToken = ""
			m.searching = true
			m.focused = focusList
			m.input.Blur()
			return m, tea.Batch(m.spinner.Tick, m.searchCmd(query, "", false))

		case key.Matches(msg, m.keys.Submit) && m.focused == focusList:
			if item, ok := m.results.SelectedItem().(videoItem); ok {
				return m, func() tea.Msg {
					return VideoSelectedMsg{Video: item.video}
				}
			}

		case key.Matches(msg, m.keys.LoadMore) && m.focused == focusList:
			return m, m.loadMore()
		}

	case SearchResultMsg:
		m.searching = false
		m.loadingMore = false
		if msg.Err != nil {
			return m, nil
		}
		m.nextToken = msg.NextToken

		if msg.Append {
			existing := m.results.Items()
			newItems := make([]list.Item, len(existing), len(existing)+len(msg.Videos))
			copy(newItems, existing)
			for _, v := range msg.Videos {
				newItems = append(newItems, videoItem{video: v})
			}
			cmd := m.results.SetItems(newItems)
			cmds = append(cmds, cmd)
		} else {
			items := make([]list.Item, len(msg.Videos))
			for i, v := range msg.Videos {
				items[i] = videoItem{video: v}
			}
			cmd := m.results.SetItems(items)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		if m.searching || m.loadingMore {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Delegate to focused component
	if m.focused == focusInput {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		var cmd tea.Cmd
		m.results, cmd = m.results.Update(msg)
		cmds = append(cmds, cmd)

		// Auto-load more when near the bottom
		total := len(m.results.Items())
		if total > 0 && m.results.Index() >= total-5 {
			cmds = append(cmds, m.loadMore())
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	inputView := inputStyle.Width(m.width).Render(m.input.View())

	if m.searching {
		return lipgloss.JoinVertical(lipgloss.Left,
			inputView,
			m.spinner.View()+" Searching...",
		)
	}

	resultsView := m.results.View()
	if m.loadingMore {
		resultsView += "\n " + m.spinner.View() + " Loading more..."
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		inputView,
		resultsView,
	)
}

func (m *Model) loadMore() tea.Cmd {
	if m.loadingMore || m.searching || m.nextToken == "" || m.query == "" {
		return nil
	}
	m.loadingMore = true
	return tea.Batch(m.spinner.Tick, m.searchCmd(m.query, m.nextToken, true))
}

func (m Model) searchCmd(query, pageToken string, isAppend bool) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		page, err := client.Search(context.Background(), query, pageToken)
		if err != nil {
			return SearchResultMsg{Err: err}
		}
		return SearchResultMsg{
			Videos:    page.Items,
			NextToken: page.NextToken,
			Append:    isAppend,
		}
	}
}

// InputFocused reports whether the text input has focus.
func (m Model) InputFocused() bool {
	return m.focused == focusInput
}

// SelectedVideo returns the currently selected video, if any.
func (m Model) SelectedVideo() (youtube.Video, bool) {
	if item, ok := m.results.SelectedItem().(videoItem); ok {
		return item.video, true
	}
	return youtube.Video{}, false
}
