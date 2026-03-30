package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/deathmaz/ytui/internal/app"
	"github.com/deathmaz/ytui/internal/config"
	"github.com/deathmaz/ytui/internal/youtube"
)

func main() {
	search := flag.String("search", "", "search query to execute on startup")
	flag.Parse()

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

	opts := app.Options{
		SearchQuery: *search,
	}
	m := app.New(client, cfg, opts)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
