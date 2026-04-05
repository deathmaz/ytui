package app

import (
	"context"
	"net/http"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/auth"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/ui/search"
	"github.com/deathmaz/ytui/internal/ui/urlinput"
	"github.com/deathmaz/ytui/internal/youtube"
)

// StatusManager handles status message display and auto-clear.
type StatusManager struct {
	Msg string
	seq int
}

// Set sets a status message and returns a command that clears it after the
// given duration. The caller should call resizeViews() afterward.
func (s *StatusManager) Set(text string, clearAfter time.Duration) tea.Cmd {
	s.seq++
	s.Msg = text
	seq := s.seq
	return tea.Tick(clearAfter, func(time.Time) tea.Msg {
		return clearStatusMsg{seq: seq}
	})
}

// SetPermanent sets a status message that won't auto-clear. Any pending
// auto-clear timer from a previous Set() is cancelled.
func (s *StatusManager) SetPermanent(text string) {
	s.seq++
	s.Msg = text
}

// Clear immediately clears the status message and cancels any pending
// auto-clear timer by bumping the sequence.
func (s *StatusManager) Clear() {
	s.seq++
	s.Msg = ""
}

// HandleClear clears the status message if the sequence matches.
// Returns true if cleared (caller should call resizeViews).
func (s *StatusManager) HandleClear(msg clearStatusMsg) bool {
	if msg.seq == s.seq {
		s.Msg = ""
		return true
	}
	return false
}

// HandleWindowSize applies common window-size fields.
func HandleWindowSize(msg tea.WindowSizeMsg, width, height *int, h *help.Model) {
	*width = msg.Width
	*height = msg.Height
	h.Width = msg.Width
}

// AuthResult is the message returned by the shared authenticate command.
type AuthResult struct {
	HTTPClient *http.Client // non-nil on success
	Err        error        // non-nil on failure
}

// TryAuthenticate checks the authenticating flag and authenticated status,
// sets the status message, and returns a command that extracts browser cookies.
// Each mode handles AuthResult in its own Update to construct its specific client.
func TryAuthenticate(authenticating *bool, isAuthenticated bool, status *StatusManager, browser string) tea.Cmd {
	if *authenticating {
		return nil
	}
	if isAuthenticated {
		return status.Set("Already authenticated", 3*time.Second)
	}
	*authenticating = true
	status.Msg = "Authenticating via " + browser + "..."
	return func() tea.Msg {
		jar, err := auth.ExtractCookies(context.Background(), browser)
		if err != nil {
			return AuthResult{Err: err}
		}
		return AuthResult{HTTPClient: auth.HTTPClient(jar)}
	}
}

// HandleAuthResult processes an AuthResult message with the shared pattern:
// check error, run mode-specific setup, set status, reload active view, handle
// pendingOpen. setupFn constructs mode-specific clients and resets state;
// reloadActiveView returns a cmd to load the currently focused view (or nil).
func HandleAuthResult(
	msg AuthResult,
	authenticating *bool,
	status *StatusManager,
	browser string,
	pendingOpen **youtube.ParsedURL,
	openFn func(*youtube.ParsedURL) tea.Cmd,
	setupFn func(*http.Client) error,
	reloadActiveView func() tea.Cmd,
	resizeViews func(),
) tea.Cmd {
	*authenticating = false
	if msg.Err != nil {
		cmd := status.Set("Auth failed: "+msg.Err.Error(), 5*time.Second)
		resizeViews()
		return cmd
	}
	if err := setupFn(msg.HTTPClient); err != nil {
		cmd := status.Set("Auth failed: "+err.Error(), 5*time.Second)
		resizeViews()
		return cmd
	}
	var cmds []tea.Cmd
	cmds = append(cmds, status.Set("Authenticated via "+browser, 3*time.Second))
	if reload := reloadActiveView(); reload != nil {
		cmds = append(cmds, reload)
	}
	if *pendingOpen != nil {
		cmds = append(cmds, openFn(*pendingOpen))
		*pendingOpen = nil
	}
	resizeViews()
	return tea.Batch(cmds...)
}

// ModalView is satisfied by overlay models like urlinput.Model and picker.Model.
type ModalView interface {
	IsActive() bool
	View() string
}

var clearKittyOnce bool

// RenderShell checks zero-width and modal overlays, then composes the standard
// tab/content/status/help layout. On the first call, it prepends a Kitty
// "delete all images" sequence to clear stale thumbnails from previous sessions.
func RenderShell(
	width int,
	modals []ModalView,
	tabsFn func() string,
	contentFn func() string,
	statusLine string,
	helpBar string,
) string {
	if width == 0 {
		return "Loading..."
	}
	for _, modal := range modals {
		if modal.IsActive() {
			return modal.View()
		}
	}
	view := composeSections(tabsFn(), contentFn(), statusLine, helpBar)
	if !clearKittyOnce {
		clearKittyOnce = true
		view = ytimage.DeleteAll() + view
	}
	return view
}

// GlobalKeyAction represents a global key that all modes handle.
type GlobalKeyAction int

const (
	KeyNotHandled  GlobalKeyAction = iota
	KeyQuit                        // ctrl+c or q
	KeyHelpToggle                  // ?
	KeyAuth                        // a
	KeyOpenURL                     // O
	KeySearch                      // / or 3
)

// HandleGlobalKey checks for keys common to all modes.
// Returns the action and true if matched, (KeyNotHandled, false) otherwise.
func HandleGlobalKey(msg tea.KeyMsg, keys KeyMap) (GlobalKeyAction, bool) {
	switch {
	case key.Matches(msg, keys.ForceQuit), key.Matches(msg, keys.Quit):
		return KeyQuit, true
	case key.Matches(msg, keys.Help):
		return KeyHelpToggle, true
	case key.Matches(msg, keys.Auth):
		return KeyAuth, true
	case key.Matches(msg, keys.OpenURL):
		return KeyOpenURL, true
	case key.Matches(msg, keys.Search):
		return KeySearch, true
	}
	return KeyNotHandled, false
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

// handleURLInput delegates to the URL input dialog when active.
// Returns (cmd, true) if handled, (nil, false) otherwise.
func handleURLInput(msg tea.Msg, u *urlinput.Model) (tea.Cmd, bool) {
	if !u.IsActive() {
		return nil, false
	}
	updated, cmd := u.Update(msg)
	*u = updated
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
