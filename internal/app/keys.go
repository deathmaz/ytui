package app

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines global keybindings.
type KeyMap struct {
	ForceQuit  key.Binding
	Quit       key.Binding
	Help       key.Binding
	Feed       key.Binding
	Subs       key.Binding
	Search     key.Binding
	Back       key.Binding
	Play       key.Binding
	PlayPick   key.Binding
	Download   key.Binding
	Detail     key.Binding
	Up         key.Binding
	Down       key.Binding
	PageUp     key.Binding
	PageDown   key.Binding
	HalfPageUp key.Binding
	HalfPageDn key.Binding
	Top        key.Binding
	Bottom     key.Binding
	Open       key.Binding
	OpenURL    key.Binding
	Yank       key.Binding
	Refresh    key.Binding
	Auth       key.Binding
	Channel    key.Binding

	// Music-specific bindings
	PlayAlbum key.Binding
	Enter     key.Binding
	NextTab   key.Binding
	PrevTab   key.Binding
	LoadMore  key.Binding
}

// DefaultKeyMap returns the default vim-like keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		ForceQuit:  key.NewBinding(key.WithKeys("ctrl+c")),
		Quit:       key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Feed:       key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "feed")),
		Subs:       key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "subscriptions")),
		Search:     key.NewBinding(key.WithKeys("3", "/"), key.WithHelp("/", "search")),
		Back:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Play:       key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "play")),
		PlayPick:   key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "play (pick quality)")),
		Download:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "download")),
		Detail:     key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "details")),
		Up:         key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "up")),
		Down:       key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "down")),
		PageUp:     key.NewBinding(key.WithKeys("ctrl+b", "pgup"), key.WithHelp("C-b", "page up")),
		PageDown:   key.NewBinding(key.WithKeys("ctrl+f", "pgdown"), key.WithHelp("C-f", "page down")),
		HalfPageUp: key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("C-u", "half page up")),
		HalfPageDn: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("C-d", "half page down")),
		Top:        key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:     key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		Open:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open in browser")),
		OpenURL:    key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "open URL")),
		Yank:       key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy URL")),
		Refresh:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Auth:       key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "authenticate")),
		Channel:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "channel")),

		PlayAlbum: key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "play album")),
		Enter:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		NextTab:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next section")),
		PrevTab:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("S-tab", "prev section")),
		LoadMore:  key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "load all")),
	}
}

// ShortHelp returns the compact help bindings.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Feed, k.Subs, k.Search, k.Help, k.Quit}
}

// FullHelp returns the extended help bindings.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDn},
		{k.Feed, k.Subs, k.Search, k.Back},
		{k.Play, k.PlayPick, k.Download, k.Detail, k.Channel},
		{k.Open, k.Yank, k.Refresh, k.Auth, k.Quit},
	}
}

// MusicShortHelp returns the compact help bindings for music mode.
func (k KeyMap) MusicShortHelp() []key.Binding {
	return []key.Binding{k.Search, k.Play, k.PlayAlbum, k.Enter, k.NextTab, k.LoadMore, k.Auth, k.OpenURL, k.Back, k.Quit}
}

// MusicFullHelp returns the extended help bindings for music mode.
func (k KeyMap) MusicFullHelp() [][]key.Binding {
	return [][]key.Binding{k.MusicShortHelp()}
}

// musicHelpAdapter wraps KeyMap to satisfy help.KeyMap with music-specific bindings.
type musicHelpAdapter struct{ KeyMap }

func (a musicHelpAdapter) ShortHelp() []key.Binding  { return a.KeyMap.MusicShortHelp() }
func (a musicHelpAdapter) FullHelp() [][]key.Binding { return a.KeyMap.MusicFullHelp() }
