package youtube

// MusicItem represents any item from a YouTube Music search/browse result.
type MusicItem struct {
	Type        MusicItemType
	Title       string
	Subtitle    string // "Album • Artist • Year" or "Song • Artist" etc.
	VideoID     string // for songs/videos (playable)
	BrowseID    string // for albums/artists/playlists (browsable)
	Thumbnails  []Thumbnail
}

// MusicItemType identifies the kind of music item.
type MusicItemType string

const (
	MusicSong     MusicItemType = "Song"
	MusicAlbum    MusicItemType = "Album"
	MusicArtist   MusicItemType = "Artist"
	MusicPlaylist MusicItemType = "Playlist"
	MusicVideo    MusicItemType = "Video"
)

// MusicShelf is a named section of music items (e.g., "Top songs", "Albums").
type MusicShelf struct {
	Title string
	Items []MusicItem
}

// MusicSearchResult holds the results of a YouTube Music search.
type MusicSearchResult struct {
	TopResult *MusicItem    // from musicCardShelfRenderer (artist/album card)
	Shelves   []MusicShelf  // from musicShelfRenderer sections
}
