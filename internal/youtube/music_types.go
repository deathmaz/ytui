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
	Title       string
	Items       []MusicItem
	MoreBrowseID string // browseId for "See all" / load more
	MoreParams   string // params for the browse call
}

// MusicSearchResult holds the results of a YouTube Music search.
type MusicSearchResult struct {
	TopResult *MusicItem    // from musicCardShelfRenderer (artist/album card)
	Shelves   []MusicShelf  // from musicShelfRenderer sections
	NextToken string        // continuation token for pagination
}

// MusicArtistPage holds data for an artist page.
type MusicArtistPage struct {
	Name     string
	Shelves  []MusicShelf // top songs, albums, singles, videos, etc.
}

// MusicAlbumPage holds data for an album page.
type MusicAlbumPage struct {
	Title       string
	Artist      string
	Year        string
	AlbumType   string      // "Album", "EP", "Single", "Playlist"
	Description string      // album description text
	TrackCount  string      // e.g. "19 songs"
	Duration    string      // e.g. "1 hour, 17 minutes"
	Thumbnails  []Thumbnail // album art
	Tracks      []MusicItem
	PlaylistID  string
}

