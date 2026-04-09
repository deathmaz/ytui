// Package state persists lightweight session state (e.g. open tabs) across
// app restarts. State files live in $XDG_STATE_HOME/ytui (or ~/.local/state/ytui).
package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Tab kind constants for persistence. Video and music modes use separate sets.
const (
	KindVideo    = "video"
	KindChannel  = "channel"
	KindPlaylist = "playlist"
	KindArtist   = "artist"
	KindAlbum    = "album"
	KindSong     = "song"
)

// TabEntry represents a single persisted tab identity.
type TabEntry struct {
	Kind  string `json:"kind"`  // one of the Kind* constants
	ID    string `json:"id"`    // video ID, channel ID, playlist ID, browseID
	Title string `json:"title"` // display title (best-effort, may be empty)
}

// TabState is the persisted tab list for a single mode.
type TabState struct {
	Tabs []TabEntry `json:"tabs"`
}

// Dir returns the ytui state directory.
func Dir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "ytui")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "ytui")
}

// Path returns the state file path for the given mode ("video" or "music").
func Path(mode string) string {
	d := Dir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, mode+"_tabs.json")
}

// Load reads the tab state for the given mode. Returns (nil, nil) if the
// file does not exist.
func Load(mode string) (*TabState, error) {
	p := Path(mode)
	if p == "" {
		return nil, nil
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s TabState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save atomically writes the tab state for the given mode. Creates the
// state directory if it does not exist.
func Save(mode string, s *TabState) error {
	p := Path(mode)
	if p == "" {
		return nil
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, mode+"_tabs-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, p)
}
