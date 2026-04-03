package youtube

import "context"

// MusicAPI defines the interface for YouTube Music data access.
type MusicAPI interface {
	IsAuthenticated() bool
	Search(ctx context.Context, query string, continuation string) (*MusicSearchResult, error)
	GetHome(ctx context.Context) ([]MusicShelf, error)
	GetLibrarySection(ctx context.Context, browseID string) (*LibrarySectionResult, error)
	GetLibraryContinuation(ctx context.Context, continuation string) (*LibrarySectionResult, error)
	GetArtist(ctx context.Context, browseID string) (*MusicArtistPage, error)
	GetAlbum(ctx context.Context, browseID string) (*MusicAlbumPage, error)
	BrowseMore(ctx context.Context, browseID, params string) ([]MusicItem, error)
	GetAlbumTracks(ctx context.Context, browseID string) ([]MusicItem, string, error)
}
