package search

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/styles"
)

var inputStyle = lipgloss.NewStyle().Padding(0, 1)

type focus int

const (
	focusInput focus = iota
	focusList
)

// SearchFunc performs a search and returns a ResultMsg.
// query is the search text. If pageToken is non-empty, it's a "load more" request.
type SearchFunc func(query, pageToken string) tea.Cmd

// SelectFunc is called when Enter is pressed on a result item.
// It should return a tea.Cmd that emits the appropriate selection message.
type SelectFunc func(item list.Item) tea.Cmd

// ResultMsg carries search results back to the model.
type ResultMsg struct {
	Items     []list.Item
	NextToken string
	Append    bool // true when loading more results
	Err       error
}

// Config holds the parameters that differ between search instances.
type Config struct {
	Placeholder string
	Delegate    list.ItemDelegate
	SearchFn    SearchFunc
	SelectFn    SelectFunc
}

// Model is a reusable search view with input, spinner, and results list.
type Model struct {
	input       textinput.Model
	results     list.Model
	spinner     spinner.Model
	keys        keyMap
	focused     focus
	searching   bool
	query       string
	nextToken   string
	loadingMore bool
	width       int
	height      int
	searchFn    SearchFunc
	selectFn    SelectFunc
}

// New creates a new search model with the given configuration.
func New(cfg Config) Model {
	ti := textinput.New()
	ti.Placeholder = cfg.Placeholder
	ti.CharLimit = 256
	ti.Focus()

	l := shared.NewList(cfg.Delegate)

	return Model{
		input:    ti,
		results:  l,
		spinner:  styles.NewSpinner(),
		keys:     defaultKeyMap(),
		focused:  focusInput,
		searchFn: cfg.SearchFn,
		selectFn: cfg.SelectFn,
	}
}

// SetSize updates the view dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	inputView := inputStyle.Width(w).Render(m.input.View())
	inputHeight := lipgloss.Height(inputView)
	m.results.SetSize(w, h-inputHeight)
	m.input.Width = w - 4
}

// Focus gives focus to the search input.
func (m *Model) Focus() {
	m.focused = focusInput
	m.input.Focus()
}

// InputFocused reports whether the text input has focus.
func (m Model) InputFocused() bool {
	return m.focused == focusInput
}

// SetQuery sets the search query and input value without executing.
func (m *Model) SetQuery(q string) {
	m.query = q
	m.input.SetValue(q)
	m.input.Blur()
	m.focused = focusList
}

// Query returns the current search query.
func (m Model) Query() string {
	return m.query
}

// Refresh re-executes the current search query.
func (m *Model) Refresh() tea.Cmd {
	if m.query == "" || m.searching {
		return nil
	}
	m.nextToken = ""
	m.searching = true
	m.results.SetItems(nil)
	m.results.ResetSelected()
	return tea.Batch(m.spinner.Tick, m.searchFn(m.query, ""))
}

// SelectedItem returns the currently selected list item.
func (m Model) SelectedItem() list.Item {
	return m.results.SelectedItem()
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.FocusInput) && m.focused == focusList:
			m.focused = focusInput
			m.input.Focus()
			return m, textinput.Blink

		case key.Matches(msg, m.keys.BlurInput) && m.focused == focusInput:
			m.focused = focusList
			m.input.Blur()
			return m, nil

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
			m.results.SetItems(nil)
			m.results.ResetSelected()
			return m, tea.Batch(m.spinner.Tick, m.searchFn(query, ""))

		case key.Matches(msg, m.keys.Submit) && m.focused == focusList:
			if item := m.results.SelectedItem(); item != nil && m.selectFn != nil {
				return m, m.selectFn(item)
			}

		case key.Matches(msg, m.keys.LoadMore) && m.focused == focusList:
			return m, m.loadMore()
		}

	case ResultMsg:
		m.searching = false
		m.loadingMore = false
		if msg.Err != nil {
			return m, nil
		}
		m.nextToken = msg.NextToken

		if msg.Append {
			existing := m.results.Items()
			items := make([]list.Item, len(existing), len(existing)+len(msg.Items))
			copy(items, existing)
			items = append(items, msg.Items...)
			cmd := m.results.SetItems(items)
			cmds = append(cmds, cmd)
		} else {
			cmd := m.results.SetItems(msg.Items)
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

// View renders the search view.
func (m Model) View() string {
	inputView := inputStyle.Width(m.width).Render(m.input.View())

	if m.searching {
		return lipgloss.JoinVertical(lipgloss.Left,
			inputView,
			m.spinner.View()+" Searching...",
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		inputView,
		m.results.View(),
	)
}

func (m *Model) loadMore() tea.Cmd {
	if m.loadingMore || m.searching || m.nextToken == "" || m.query == "" {
		return nil
	}
	m.loadingMore = true
	return tea.Batch(m.spinner.Tick, m.searchFn(m.query, m.nextToken))
}
