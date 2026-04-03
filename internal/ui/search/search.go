package search

import (
	"context"

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

// SearchResult is the concrete return type for search operations.
// Callers must populate Items and NextToken — the compiler enforces
// that these fields exist even if empty.
type SearchResult struct {
	Items     []list.Item
	NextToken string
	Err       error
}

// SearchFunc performs a search. The search model calls this in a goroutine
// and converts the result into an internal message. Callers cannot return
// an arbitrary tea.Msg — they must return a SearchResult.
type SearchFunc func(ctx context.Context, query, pageToken string) SearchResult

// SelectFunc is called synchronously when Enter is pressed on a result.
// Returns the message to emit, or nil for no action.
type SelectFunc func(item list.Item) tea.Msg

// resultMsg is internal — only created by the search model from SearchResult.
type resultMsg struct {
	result SearchResult
	append bool
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
	if cfg.SearchFn == nil {
		panic("search.Config.SearchFn must not be nil")
	}

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

// Blur removes focus from the search input.
func (m *Model) Blur() {
	m.focused = focusList
	m.input.Blur()
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
	return tea.Batch(m.spinner.Tick, m.searchCmd(m.query, "", false))
}

// SelectedItem returns the currently selected list item.
func (m Model) SelectedItem() list.Item {
	return m.results.SelectedItem()
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	if m.focused == focusInput {
		return textinput.Blink
	}
	return nil
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
			return m, tea.Batch(m.spinner.Tick, m.searchCmd(query, "", false))

		case key.Matches(msg, m.keys.Submit) && m.focused == focusList:
			if item := m.results.SelectedItem(); item != nil && m.selectFn != nil {
				selMsg := m.selectFn(item)
				if selMsg != nil {
					return m, func() tea.Msg { return selMsg }
				}
			}

		case key.Matches(msg, m.keys.LoadMore) && m.focused == focusList:
			return m, m.loadMore()
		}

	case resultMsg:
		m.searching = false
		m.loadingMore = false
		if msg.result.Err != nil {
			return m, nil
		}
		m.nextToken = msg.result.NextToken

		if msg.append {
			existing := m.results.Items()
			items := make([]list.Item, len(existing), len(existing)+len(msg.result.Items))
			copy(items, existing)
			items = append(items, msg.result.Items...)
			cmd := m.results.SetItems(items)
			cmds = append(cmds, cmd)
		} else {
			cmd := m.results.SetItems(msg.result.Items)
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

		if shared.ShouldLoadMore(m.results, 5) {
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

// searchCmd wraps SearchFunc into a tea.Cmd with the internal resultMsg.
func (m *Model) searchCmd(query, pageToken string, isAppend bool) tea.Cmd {
	fn := m.searchFn
	return func() tea.Msg {
		result := fn(context.Background(), query, pageToken)
		return resultMsg{result: result, append: isAppend}
	}
}

func (m *Model) loadMore() tea.Cmd {
	if m.loadingMore || m.searching || m.nextToken == "" || m.query == "" {
		return nil
	}
	m.loadingMore = true
	return tea.Batch(m.spinner.Tick, m.searchCmd(m.query, m.nextToken, true))
}
