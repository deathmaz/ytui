package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/deathmaz/ytui/internal/app"
	"github.com/deathmaz/ytui/internal/config"
	"github.com/deathmaz/ytui/internal/youtube"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	client, err := youtube.NewInnerTubeClient(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating youtube client: %v\n", err)
		os.Exit(1)
	}

	m := app.New(client, cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
