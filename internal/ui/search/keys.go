package search

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Submit     key.Binding
	FocusInput key.Binding
	BlurInput  key.Binding
	LoadMore   key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Submit:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search/select")),
		FocusInput: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "focus search")),
		BlurInput:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "unfocus")),
		LoadMore:   key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "load more")),
	}
}
