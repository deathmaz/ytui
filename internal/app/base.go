package app

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/ui/search"
	"github.com/deathmaz/ytui/internal/youtube"
)

// setStatusCmd sets a status message and returns a command that clears it
// after the given duration. The caller should call resizeViews() afterward.
func setStatusCmd(seq *int, statusMsg *string, text string, clearAfter time.Duration) tea.Cmd {
	*seq++
	*statusMsg = text
	s := *seq
	return tea.Tick(clearAfter, func(time.Time) tea.Msg {
		return clearStatusMsg{seq: s}
	})
}

// handleClearStatus clears the status message if the sequence matches.
// Returns true if cleared (caller should call resizeViews).
func handleClearStatus(msg clearStatusMsg, seq int, statusMsg *string) bool {
	if msg.seq == seq {
		*statusMsg = ""
		return true
	}
	return false
}

// calcContentHeight computes available content height given rendered
// tabs and help views, plus whether a status line is shown.
func calcContentHeight(totalHeight int, tabsView, helpView string, hasStatusLine bool) int {
	overhead := lipgloss.Height(tabsView) + lipgloss.Height(helpView)
	if hasStatusLine {
		overhead++
	}
	h := totalHeight - overhead
	if h < 1 {
		h = 1
	}
	return h
}

// composeSections joins tabs, content, an optional status line, and help
// into a single vertical layout.
func composeSections(tabsView, contentView, statusLine, helpView string) string {
	var sections []string
	sections = append(sections, tabsView, contentView)
	if statusLine != "" {
		sections = append(sections, statusLine)
	}
	sections = append(sections, helpView)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// handleSearchFocused delegates to the search model when its input is focused.
// Returns (cmd, true) if the message was consumed, (nil, false) otherwise.
func handleSearchFocused(msg tea.Msg, s *search.Model, searchActive bool, keys KeyMap) (tea.Cmd, bool) {
	if !searchActive || !s.InputFocused() {
		return nil, false
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	if key.Matches(keyMsg, keys.ForceQuit) || key.Matches(keyMsg, keys.Quit) {
		return tea.Quit, true
	}
	updated, cmd := s.Update(msg)
	*s = updated
	return cmd, true
}

// initCmds builds the standard Init command batch with auth-then-open logic.
func initCmds(
	authOnStartup bool,
	pendingOpen **youtube.ParsedURL,
	searchInit tea.Cmd,
	authCmd func() tea.Cmd,
	openFn func(*youtube.ParsedURL) tea.Cmd,
	searchQuery string,
	refreshCmd func() tea.Cmd,
) tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, searchInit)
	if authOnStartup {
		cmds = append(cmds, authCmd())
		if *pendingOpen != nil {
			return tea.Batch(cmds...)
		}
	}
	if *pendingOpen != nil {
		cmds = append(cmds, openFn(*pendingOpen))
		*pendingOpen = nil
	}
	if searchQuery != "" {
		cmds = append(cmds, refreshCmd())
	}
	return tea.Batch(cmds...)
}
