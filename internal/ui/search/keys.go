package search

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Submit     key.Binding
	FocusInput key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Submit:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search/select")),
		FocusInput: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "focus search")),
	}
}
