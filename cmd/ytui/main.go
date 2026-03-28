package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/deathmaz/ytui/internal/app"
	"github.com/deathmaz/ytui/internal/youtube"
)

func main() {
	client, err := youtube.NewInnerTubeClient(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating youtube client: %v\n", err)
		os.Exit(1)
	}

	m := app.New(client)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
