package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/deathmaz/ytui/internal/app"
	"github.com/deathmaz/ytui/internal/config"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/youtube"
)

func main() {
	search := flag.String("search", "", "search query to execute on startup")
	music := flag.Bool("music", false, "start in YouTube Music mode")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// CLI flag overrides config
	if *music {
		cfg.General.Mode = "music"
	}

	opts := app.Options{
		SearchQuery: *search,
	}

	var m tea.Model
	if cfg.General.Mode == "music" {
		mc, err := youtube.NewMusicClient(nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating music client: %v\n", err)
			os.Exit(1)
		}
		ytClient, err := youtube.NewInnerTubeClient(nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating youtube client: %v\n", err)
			os.Exit(1)
		}
		imgR := ytimage.NewRenderer()
		m = app.NewMusic(mc, ytClient, cfg, imgR, opts)
	} else {
		client, err := youtube.NewInnerTubeClient(nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating youtube client: %v\n", err)
			os.Exit(1)
		}
		m = app.New(client, cfg, opts)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
