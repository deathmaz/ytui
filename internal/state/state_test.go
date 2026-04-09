package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDir_XDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/fake_state")
	got := Dir()
	if got != "/tmp/fake_state/ytui" {
		t.Errorf("Dir() = %q, want %q", got, "/tmp/fake_state/ytui")
	}
}

func TestDir_FallbackHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	got := Dir()
	if got == "" {
		t.Error("Dir() should not be empty when HOME is set")
	}
	if !strings.HasSuffix(got, "/.local/state/ytui") {
		t.Errorf("Dir() = %q, want suffix %q", got, "/.local/state/ytui")
	}
}

func TestPath(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/fake_state")
	got := Path("video")
	if got != "/tmp/fake_state/ytui/video_tabs.json" {
		t.Errorf("Path(video) = %q, want %q", got, "/tmp/fake_state/ytui/video_tabs.json")
	}
	got = Path("music")
	if got != "/tmp/fake_state/ytui/music_tabs.json" {
		t.Errorf("Path(music) = %q, want %q", got, "/tmp/fake_state/ytui/music_tabs.json")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	want := &TabState{
		Tabs: []TabEntry{
			{Kind: KindVideo, ID: "fake_vid_001", Title: "Fake Video"},
			{Kind: KindChannel, ID: "UCfake123", Title: "Fake Channel"},
		},
	}

	if err := Save("video", want); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, err := Load("video")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got == nil {
		t.Fatal("Load() returned nil")
	}
	if len(got.Tabs) != len(want.Tabs) {
		t.Fatalf("Load() returned %d tabs, want %d", len(got.Tabs), len(want.Tabs))
	}
	for i, tab := range got.Tabs {
		if tab != want.Tabs[i] {
			t.Errorf("tab[%d] = %+v, want %+v", i, tab, want.Tabs[i])
		}
	}
}

func TestLoad_NoFile(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	got, err := Load("video")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got != nil {
		t.Errorf("Load() = %+v, want nil", got)
	}
}

func TestLoad_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	stateDir := filepath.Join(dir, "ytui")
	os.MkdirAll(stateDir, 0755)
	os.WriteFile(filepath.Join(stateDir, "video_tabs.json"), []byte("not json{{{"), 0644)

	got, err := Load("video")
	if err == nil {
		t.Error("expected error for corrupt JSON")
	}
	if got != nil {
		t.Errorf("Load() = %+v, want nil on error", got)
	}
}

func TestSave_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	// ytui subdir doesn't exist yet
	s := &TabState{Tabs: []TabEntry{{Kind: KindVideo, ID: "fake_001", Title: "Test"}}}
	if err := Save("video", s); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify file exists
	p := Path("video")
	if _, err := os.Stat(p); err != nil {
		t.Errorf("state file not created: %v", err)
	}
}

func TestSave_EmptyTabs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	s := &TabState{Tabs: []TabEntry{}}
	if err := Save("video", s); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, err := Load("video")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got == nil {
		t.Fatal("Load() returned nil for empty tabs")
	}
	if len(got.Tabs) != 0 {
		t.Errorf("Load() returned %d tabs, want 0", len(got.Tabs))
	}
}

func TestIndependentModes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	videoState := &TabState{Tabs: []TabEntry{{Kind: KindVideo, ID: "fake_vid_001", Title: "Video"}}}
	musicState := &TabState{Tabs: []TabEntry{{Kind: KindArtist, ID: "UCfake_artist", Title: "Artist"}}}

	if err := Save("video", videoState); err != nil {
		t.Fatalf("Save(video) error: %v", err)
	}
	if err := Save("music", musicState); err != nil {
		t.Fatalf("Save(music) error: %v", err)
	}

	gotVideo, err := Load("video")
	if err != nil {
		t.Fatalf("Load(video) error: %v", err)
	}
	gotMusic, err := Load("music")
	if err != nil {
		t.Fatalf("Load(music) error: %v", err)
	}

	if len(gotVideo.Tabs) != 1 || gotVideo.Tabs[0].Kind != "video" {
		t.Errorf("video state = %+v, want video tab", gotVideo.Tabs)
	}
	if len(gotMusic.Tabs) != 1 || gotMusic.Tabs[0].Kind != "artist" {
		t.Errorf("music state = %+v, want artist tab", gotMusic.Tabs)
	}
}
