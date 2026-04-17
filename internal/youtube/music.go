package youtube

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	innertubego "github.com/nezbut/innertube-go"
	"github.com/tidwall/gjson"
)

// WEB_REMIX client static config. Version and API key are scraped at startup.
const (
	musicClientName = "WEB_REMIX"
	musicClientID   = 67
	musicReferer    = "https://music.youtube.com/"
)

func newInnerTubeMusic(httpClient *http.Client) *innertubego.InnerTube {
	return &innertubego.InnerTube{
		Adaptor: innertubego.NewInnerTubeAdaptor(innertubego.ClientContext{
			ClientName:    musicClientName,
			ClientVersion: clientParams.music.ClientVersion,
			ClientID:      musicClientID,
			APIKey:        clientParams.music.APIKey,
			UserAgent:     defaultUserAgent,
			Referer:       musicReferer,
		}, httpClient),
	}
}

// MusicClient provides YouTube Music API access via InnerTube WEB_REMIX client.
type MusicClient struct {
	mu            sync.Mutex
	it            *innertubego.InnerTube
	authenticated bool
}

// NewMusicClient creates a new YouTube Music client.
// Pass a custom httpClient with a cookie jar for authenticated requests.
func NewMusicClient(httpClient *http.Client) (*MusicClient, error) {
	it := newInnerTubeMusic(httpClient)
	return &MusicClient{it: it, authenticated: httpClient != nil && httpClient.Jar != nil}, nil
}

// IsAuthenticated reports whether the client has valid credentials.
func (c *MusicClient) IsAuthenticated() bool {
	return c.authenticated
}

// getMusicBrowseTabs resolves the tabs array from either single or two-column browse responses.
func getMusicBrowseTabs(data gjson.Result) gjson.Result {
	tabs := data.Get("contents.singleColumnBrowseResultsRenderer.tabs")
	if !tabs.Exists() {
		tabs = data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	}
	return tabs
}

// Search searches YouTube Music for the given query.
// If continuation is non-empty, it fetches the next page of results.
func (c *MusicClient) Search(ctx context.Context, query string, continuation string) (*MusicSearchResult, error) {
	c.mu.Lock()
	var cont *string
	if continuation != "" {
		cont = &continuation
	}
	raw, err := c.it.Search(ctx, &query, nil, cont)
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("music search: %w", err)
	}
	return parseMusicSearchResponse(raw)
}

// Browse fetches a YouTube Music browse page (home, album, artist, playlist).
func (c *MusicClient) Browse(ctx context.Context, browseID string) (map[string]interface{}, error) {
	c.mu.Lock()
	raw, err := c.it.Browse(ctx, &browseID, nil, nil)
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("music browse: %w", err)
	}
	return raw, nil
}

// GetHome fetches the YouTube Music home feed.
func (c *MusicClient) GetHome(ctx context.Context) ([]MusicShelf, error) {
	browseID := "FEmusic_home"
	raw, err := c.Browse(ctx, browseID)
	if err != nil {
		return nil, err
	}
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	var shelves []MusicShelf
	tabs := getMusicBrowseTabs(data)
	tabs.ForEach(func(_, tab gjson.Result) bool {
		sections := tab.Get("tabRenderer.content.sectionListRenderer.contents")
		sections.ForEach(func(_, section gjson.Result) bool {
			carousel := section.Get("musicCarouselShelfRenderer")
			if !carousel.Exists() {
				return true
			}
			title := carousel.Get("header.musicCarouselShelfBasicHeaderRenderer.title.runs.0.text").String()
			var items []MusicItem
			carousel.Get("contents").ForEach(func(_, entry gjson.Result) bool {
				mtrir := entry.Get("musicTwoRowItemRenderer")
				if mtrir.Exists() {
					items = append(items, parseMusicTwoRowItem(mtrir))
				}
				mrlir := entry.Get("musicResponsiveListItemRenderer")
				if mrlir.Exists() {
					items = append(items, parseMusicListItem(mrlir))
				}
				return true
			})
			if len(items) > 0 {
				shelves = append(shelves, MusicShelf{Title: title, Items: items})
			}
			return true
		})
		return true
	})

	return shelves, nil
}

// LibrarySection identifies a section of the user's YouTube Music library.
type LibrarySection struct {
	Title    string
	BrowseID string
}

// LibrarySections lists all fetchable library sections.
var LibrarySections = []LibrarySection{
	{"Playlists", "FEmusic_liked_playlists"},
	{"Songs", "FEmusic_liked_videos"},
	{"Albums", "FEmusic_liked_albums"},
	{"Subscriptions", "FEmusic_library_corpus_artists"},
}

// LibrarySectionResult holds items and an optional continuation token.
type LibrarySectionResult struct {
	Items        []MusicItem
	Continuation string
}

// GetLibrarySection fetches a single library section by browseID.
func (c *MusicClient) GetLibrarySection(ctx context.Context, browseID string) (*LibrarySectionResult, error) {
	raw, err := c.Browse(ctx, browseID)
	if err != nil {
		return nil, err
	}
	return parseLibrarySection(raw)
}

// GetLibraryContinuation fetches the next page of a library section.
func (c *MusicClient) GetLibraryContinuation(ctx context.Context, continuation string) (*LibrarySectionResult, error) {
	c.mu.Lock()
	raw, err := c.it.Browse(ctx, nil, nil, &continuation)
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("library continuation: %w", err)
	}
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	result := &LibrarySectionResult{}
	// Continuation responses use appendContinuationItemsAction or musicShelfContinuation
	data.Get("continuationContents.musicShelfContinuation.contents").ForEach(func(_, entry gjson.Result) bool {
		mrlir := entry.Get("musicResponsiveListItemRenderer")
		if mrlir.Exists() {
			result.Items = append(result.Items, parseMusicListItem(mrlir))
		}
		return true
	})
	ct := data.Get("continuationContents.musicShelfContinuation.continuations.0.nextContinuationData.continuation").String()
	if ct != "" {
		result.Continuation = ct
	}

	// Grid continuation
	data.Get("continuationContents.gridContinuation.items").ForEach(func(_, entry gjson.Result) bool {
		mtrir := entry.Get("musicTwoRowItemRenderer")
		if mtrir.Exists() {
			item := parseMusicTwoRowItem(mtrir)
			if item.BrowseID != "" || item.VideoID != "" {
				result.Items = append(result.Items, item)
			}
		}
		return true
	})
	ct2 := data.Get("continuationContents.gridContinuation.continuations.0.nextContinuationData.continuation").String()
	if ct2 != "" {
		result.Continuation = ct2
	}

	return result, nil
}

func parseLibrarySection(raw map[string]interface{}) (*LibrarySectionResult, error) {
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	result := &LibrarySectionResult{}
	tabs := getMusicBrowseTabs(data)
	tabs.ForEach(func(_, tab gjson.Result) bool {
		sections := tab.Get("tabRenderer.content.sectionListRenderer.contents")
		sections.ForEach(func(_, section gjson.Result) bool {
			// Grid items (playlists, albums)
			grid := section.Get("gridRenderer")
			grid.Get("items").ForEach(func(_, entry gjson.Result) bool {
				mtrir := entry.Get("musicTwoRowItemRenderer")
				if mtrir.Exists() {
					item := parseMusicTwoRowItem(mtrir)
					if item.BrowseID == "" && item.VideoID == "" {
						return true
					}
					result.Items = append(result.Items, item)
				}
				return true
			})
			ct := grid.Get("continuations.0.nextContinuationData.continuation").String()
			if ct != "" {
				result.Continuation = ct
			}

			// Shelf items (songs, artists)
			shelf := section.Get("musicShelfRenderer")
			shelf.Get("contents").ForEach(func(_, entry gjson.Result) bool {
				mrlir := entry.Get("musicResponsiveListItemRenderer")
				if mrlir.Exists() {
					item := parseMusicListItem(mrlir)
					// Skip UI-only items like "Shuffle all"
					if item.BrowseID == "" && item.VideoID == "" {
						return true
					}
					result.Items = append(result.Items, item)
				}
				return true
			})
			ct2 := shelf.Get("continuations.0.nextContinuationData.continuation").String()
			if ct2 != "" {
				result.Continuation = ct2
			}
			return true
		})
		return true
	})

	return result, nil
}

// GetArtist fetches an artist page.
func (c *MusicClient) GetArtist(ctx context.Context, browseID string) (*MusicArtistPage, error) {
	raw, err := c.Browse(ctx, browseID)
	if err != nil {
		return nil, err
	}
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	page := &MusicArtistPage{}

	// Header
	header := data.Get("header.musicImmersiveHeaderRenderer")
	if header.Exists() {
		page.Name = header.Get("title.runs.0.text").String()
	}

	// Content sections
	tabs := getMusicBrowseTabs(data)
	tabs.ForEach(func(_, tab gjson.Result) bool {
		sections := tab.Get("tabRenderer.content.sectionListRenderer.contents")
		sections.ForEach(func(_, section gjson.Result) bool {
			// Top songs shelf
			shelf := section.Get("musicShelfRenderer")
			if shelf.Exists() {
				title := shelf.Get("title.runs.0.text").String()
				var items []MusicItem
				shelf.Get("contents").ForEach(func(_, entry gjson.Result) bool {
					mrlir := entry.Get("musicResponsiveListItemRenderer")
					if mrlir.Exists() {
						items = append(items, parseMusicListItem(mrlir))
					}
					return true
				})
				ms := MusicShelf{Title: title, Items: items}
				be := shelf.Get("bottomEndpoint.browseEndpoint")
				if be.Exists() {
					ms.MoreBrowseID = be.Get("browseId").String()
					ms.MoreParams = be.Get("params").String()
				}
				page.Shelves = append(page.Shelves, ms)
			}

			// Carousels (albums, singles, videos)
			carousel := section.Get("musicCarouselShelfRenderer")
			if carousel.Exists() {
				hdr := carousel.Get("header.musicCarouselShelfBasicHeaderRenderer")
				title := hdr.Get("title.runs.0.text").String()
				var items []MusicItem
				carousel.Get("contents").ForEach(func(_, entry gjson.Result) bool {
					mtrir := entry.Get("musicTwoRowItemRenderer")
					if mtrir.Exists() {
						items = append(items, parseMusicTwoRowItem(mtrir))
					}
					mrlir := entry.Get("musicResponsiveListItemRenderer")
					if mrlir.Exists() {
						items = append(items, parseMusicListItem(mrlir))
					}
					return true
				})
				ms := MusicShelf{Title: title, Items: items}
				mcb := hdr.Get("moreContentButton.buttonRenderer.navigationEndpoint.browseEndpoint")
				if mcb.Exists() {
					ms.MoreBrowseID = mcb.Get("browseId").String()
					ms.MoreParams = mcb.Get("params").String()
				}
				page.Shelves = append(page.Shelves, ms)
			}
			return true
		})
		return true
	})

	return page, nil
}

// GetAlbum fetches an album page with tracks.
func (c *MusicClient) GetAlbum(ctx context.Context, browseID string) (*MusicAlbumPage, error) {
	raw, err := c.Browse(ctx, browseID)
	if err != nil {
		return nil, err
	}
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	page := &MusicAlbumPage{}

	// Header
	header := data.Get("header.musicImmersiveHeaderRenderer")
	if !header.Exists() {
		header = data.Get("header.musicDetailHeaderRenderer")
	}
	if !header.Exists() {
		header = data.Get("contents.twoColumnBrowseResultsRenderer.tabs.0.tabRenderer.content.sectionListRenderer.contents.0.musicResponsiveHeaderRenderer")
	}
	if header.Exists() {
		page.Title = header.Get("title.runs.0.text").String()

		// Subtitle: album type, artist, year
		firstRun := true
		header.Get("subtitle.runs").ForEach(func(_, run gjson.Result) bool {
			text := run.Get("text").String()
			if firstRun {
				firstRun = false
				switch text {
				case "Album", "EP", "Single", "Playlist":
					page.AlbumType = text
					return true
				}
			}
			if page.Artist == "" && text != " • " {
				page.Artist = text
			}
			if len(text) == 4 && text[0] >= '1' && text[0] <= '2' {
				page.Year = text
			}
			return true
		})

		// Description
		var descParts []string
		header.Get("description.runs").ForEach(func(_, run gjson.Result) bool {
			descParts = append(descParts, run.Get("text").String())
			return true
		})
		if len(descParts) > 0 {
			page.Description = strings.Join(descParts, "")
		}

		// secondSubtitle: track count (run 0) and duration (run 2)
		secondSub := header.Get("secondSubtitle.runs")
		if secondSub.Exists() {
			runs := secondSub.Array()
			if len(runs) > 0 {
				page.TrackCount = runs[0].Get("text").String()
			}
			if len(runs) > 2 {
				page.Duration = runs[2].Get("text").String()
			}
		}

		// Thumbnails — try multiple paths
		thumbPath := header.Get("thumbnail.croppedSquareThumbnailRenderer.thumbnail.thumbnails")
		if !thumbPath.Exists() {
			thumbPath = header.Get("thumbnail.musicThumbnailRenderer.thumbnail.thumbnails")
		}
		if !thumbPath.Exists() {
			thumbPath = header.Get("thumbnail.thumbnails")
		}
		page.Thumbnails = parseThumbnails(thumbPath)
	}

	// Helper to extract tracks from a shelf (musicShelfRenderer or musicPlaylistShelfRenderer)
	parseShelf := func(shelf gjson.Result) {
		shelf.Get("contents").ForEach(func(_, entry gjson.Result) bool {
			mrlir := entry.Get("musicResponsiveListItemRenderer")
			if !mrlir.Exists() {
				return true
			}
			title := mrlir.Get("flexColumns.0.musicResponsiveListItemFlexColumnRenderer.text.runs.0.text").String()
			vid := mrlir.Get("overlay.musicItemThumbnailOverlayRenderer.content.musicPlayButtonRenderer.playNavigationEndpoint.watchEndpoint.videoId").String()
			plid := mrlir.Get("overlay.musicItemThumbnailOverlayRenderer.content.musicPlayButtonRenderer.playNavigationEndpoint.watchEndpoint.playlistId").String()

			if page.PlaylistID == "" && plid != "" {
				page.PlaylistID = plid
			}

			dur := mrlir.Get("fixedColumns.0.musicResponsiveListItemFixedColumnRenderer.text.runs.0.text").String()

			page.Tracks = append(page.Tracks, MusicItem{
				Type:     MusicSong,
				Title:    title,
				VideoID:  vid,
				Subtitle: dur,
			})
			return true
		})
	}

	// Try secondaryContents first (albums use this)
	sections := data.Get("contents.twoColumnBrowseResultsRenderer.secondaryContents.sectionListRenderer.contents")
	sections.ForEach(func(_, section gjson.Result) bool {
		if shelf := section.Get("musicShelfRenderer"); shelf.Exists() {
			parseShelf(shelf)
			return false
		}
		if shelf := section.Get("musicPlaylistShelfRenderer"); shelf.Exists() {
			parseShelf(shelf)
			return false
		}
		return true
	})

	// Fallback: playlists may use singleColumnBrowseResultsRenderer with tabs
	if len(page.Tracks) == 0 {
		tabs := data.Get("contents.singleColumnBrowseResultsRenderer.tabs")
		if !tabs.Exists() {
			tabs = data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
		}
		tabs.ForEach(func(_, tab gjson.Result) bool {
			tab.Get("tabRenderer.content.sectionListRenderer.contents").ForEach(func(_, section gjson.Result) bool {
				if shelf := section.Get("musicPlaylistShelfRenderer"); shelf.Exists() {
					parseShelf(shelf)
					return false
				}
				if shelf := section.Get("musicShelfRenderer"); shelf.Exists() {
					parseShelf(shelf)
					return false
				}
				return true
			})
			return len(page.Tracks) == 0
		})
	}

	return page, nil
}

func parseMusicTwoRowItem(mtrir gjson.Result) MusicItem {
	title := mtrir.Get("title.runs.0.text").String()
	var subtitleParts []string
	mtrir.Get("subtitle.runs").ForEach(func(_, r gjson.Result) bool {
		subtitleParts = append(subtitleParts, r.Get("text").String())
		return true
	})
	subtitle := strings.Join(subtitleParts, "")
	browseID := mtrir.Get("navigationEndpoint.browseEndpoint.browseId").String()

	// videoId can be in multiple locations depending on item type
	videoID := mtrir.Get("overlay.musicItemThumbnailOverlayRenderer.content.musicPlayButtonRenderer.playNavigationEndpoint.watchEndpoint.videoId").String()
	if videoID == "" {
		videoID = mtrir.Get("navigationEndpoint.watchEndpoint.videoId").String()
	}
	if videoID == "" {
		videoID = mtrir.Get("thumbnailOverlay.musicItemThumbnailOverlayRenderer.content.musicPlayButtonRenderer.playNavigationEndpoint.watchEndpoint.videoId").String()
	}

	itemType := detectMusicItemType(subtitle, browseID)

	item := MusicItem{
		Type:     itemType,
		Title:    title,
		Subtitle: subtitle,
		BrowseID: browseID,
		VideoID:  videoID,
	}

	item.Thumbnails = parseThumbnails(mtrir.Get("thumbnailRenderer.musicThumbnailRenderer.thumbnail.thumbnails"))

	return item
}

// BrowseMore fetches a "See all" page (e.g., all albums for an artist).
// Returns the items from gridRenderer or musicShelfRenderer.
func (c *MusicClient) BrowseMore(ctx context.Context, browseID, params string) ([]MusicItem, error) {
	c.mu.Lock()
	raw, err := c.it.Browse(ctx, &browseID, &params, nil)
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("music browse more: %w", err)
	}
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	var items []MusicItem
	tabs := getMusicBrowseTabs(data)
	tabs.ForEach(func(_, tab gjson.Result) bool {
		sections := tab.Get("tabRenderer.content.sectionListRenderer.contents")
		sections.ForEach(func(_, section gjson.Result) bool {
			// gridRenderer (albums, singles)
			section.Get("gridRenderer.items").ForEach(func(_, entry gjson.Result) bool {
				mtrir := entry.Get("musicTwoRowItemRenderer")
				if mtrir.Exists() {
					items = append(items, parseMusicTwoRowItem(mtrir))
				}
				return true
			})
			// musicShelfRenderer (songs, playlists)
			section.Get("musicShelfRenderer.contents").ForEach(func(_, entry gjson.Result) bool {
				mrlir := entry.Get("musicResponsiveListItemRenderer")
				if mrlir.Exists() {
					items = append(items, parseMusicListItem(mrlir))
				}
				return true
			})
			// musicPlaylistShelfRenderer
			section.Get("musicPlaylistShelfRenderer.contents").ForEach(func(_, entry gjson.Result) bool {
				mrlir := entry.Get("musicResponsiveListItemRenderer")
				if mrlir.Exists() {
					items = append(items, parseMusicListItem(mrlir))
				}
				return true
			})
			return true
		})
		return true
	})

	return items, nil
}

// GetAlbumTracks fetches the tracks for an album browseID.
// Returns the tracks and playlistID for playback.
func (c *MusicClient) GetAlbumTracks(ctx context.Context, browseID string) ([]MusicItem, string, error) {
	album, err := c.GetAlbum(ctx, browseID)
	if err != nil {
		return nil, "", err
	}
	return album.Tracks, album.PlaylistID, nil
}

func parseMusicSearchResponse(raw map[string]interface{}) (*MusicSearchResult, error) {
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	result := &MusicSearchResult{}

	// Initial response
	sections := data.Get("contents.tabbedSearchResultsRenderer.tabs.0.tabRenderer.content.sectionListRenderer.contents")
	sections.ForEach(func(_, section gjson.Result) bool {
		card := section.Get("musicCardShelfRenderer")
		if card.Exists() {
			item := parseMusicCard(card)
			result.TopResult = &item
			return true
		}

		shelf := section.Get("musicShelfRenderer")
		if shelf.Exists() {
			title := shelf.Get("title.runs.0.text").String()
			var items []MusicItem
			shelf.Get("contents").ForEach(func(_, entry gjson.Result) bool {
				mrlir := entry.Get("musicResponsiveListItemRenderer")
				if mrlir.Exists() {
					items = append(items, parseMusicListItem(mrlir))
				}
				return true
			})
			result.Shelves = append(result.Shelves, MusicShelf{
				Title: title,
				Items: items,
			})
		}

		// Continuation token
		ci := section.Get("continuationItemRenderer")
		if ci.Exists() {
			result.NextToken = extractContinuationToken(ci)
		}
		return true
	})

	// Continuation response
	data.Get("continuationContents.musicShelfContinuation.contents").ForEach(func(_, entry gjson.Result) bool {
		mrlir := entry.Get("musicResponsiveListItemRenderer")
		if mrlir.Exists() {
			result.Shelves = append(result.Shelves, MusicShelf{
				Items: []MusicItem{parseMusicListItem(mrlir)},
			})
		}
		return true
	})
	ct := data.Get("continuationContents.musicShelfContinuation.continuations.0.nextContinuationData.continuation").String()
	if ct != "" {
		result.NextToken = ct
	}

	return result, nil
}

func parseMusicCard(card gjson.Result) MusicItem {
	title := card.Get("title.runs.0.text").String()
	browseID := card.Get("title.runs.0.navigationEndpoint.browseEndpoint.browseId").String()

	var subtitleParts []string
	card.Get("subtitle.runs").ForEach(func(_, r gjson.Result) bool {
		subtitleParts = append(subtitleParts, r.Get("text").String())
		return true
	})
	subtitle := strings.Join(subtitleParts, "")

	itemType := detectMusicItemType(subtitle, browseID)

	item := MusicItem{
		Type:     itemType,
		Title:    title,
		Subtitle: subtitle,
		BrowseID: browseID,
	}

	item.Thumbnails = parseThumbnails(card.Get("thumbnail.musicThumbnailRenderer.thumbnail.thumbnails"))

	return item
}

func parseMusicListItem(mrlir gjson.Result) MusicItem {
	cols := mrlir.Get("flexColumns")

	// Column 0: title
	title := cols.Get("0.musicResponsiveListItemFlexColumnRenderer.text.runs.0.text").String()

	// Column 1: subtitle with type info (e.g., "Song • Artist" or "Album • Artist • Year")
	var subtitleParts []string
	cols.Get("1.musicResponsiveListItemFlexColumnRenderer.text.runs").ForEach(func(_, r gjson.Result) bool {
		subtitleParts = append(subtitleParts, r.Get("text").String())
		return true
	})
	subtitle := strings.Join(subtitleParts, "")

	// Get videoId (for playable items)
	videoID := mrlir.Get("overlay.musicItemThumbnailOverlayRenderer.content.musicPlayButtonRenderer.playNavigationEndpoint.watchEndpoint.videoId").String()

	// Get browseId (for browsable items) — check multiple locations
	browseID := mrlir.Get("navigationEndpoint.browseEndpoint.browseId").String()
	if browseID == "" {
		// Library artists/subscriptions have browseId in the title run's navigation
		browseID = cols.Get("0.musicResponsiveListItemFlexColumnRenderer.text.runs.0.navigationEndpoint.browseEndpoint.browseId").String()
	}

	// Detect type: try subtitle first word, then fall back to browseID-based detection
	itemType := MusicSong
	detected := false
	if len(subtitleParts) > 0 {
		first := strings.TrimSpace(subtitleParts[0])
		switch first {
		case "Album":
			itemType = MusicAlbum
			detected = true
		case "Artist":
			itemType = MusicArtist
			detected = true
		case "Playlist":
			itemType = MusicPlaylist
			detected = true
		case "Video":
			itemType = MusicVideo
			detected = true
		case "Song":
			itemType = MusicSong
			detected = true
		}
	}
	if !detected {
		itemType = detectMusicItemType(subtitle, browseID)
	}

	item := MusicItem{
		Type:     itemType,
		Title:    title,
		Subtitle: subtitle,
		VideoID:  videoID,
		BrowseID: browseID,
	}

	item.Thumbnails = parseThumbnails(mrlir.Get("thumbnail.musicThumbnailRenderer.thumbnail.thumbnails"))

	return item
}

func parseThumbnails(arr gjson.Result) []Thumbnail {
	var thumbs []Thumbnail
	arr.ForEach(func(_, t gjson.Result) bool {
		thumbs = append(thumbs, Thumbnail{
			URL:    t.Get("url").String(),
			Width:  int(t.Get("width").Int()),
			Height: int(t.Get("height").Int()),
		})
		return true
	})
	return thumbs
}

func detectMusicItemType(subtitle, browseID string) MusicItemType {
	// Check browseID prefix first (most reliable)
	if strings.HasPrefix(browseID, "UC") {
		return MusicArtist
	}
	if strings.HasPrefix(browseID, "MPRE") || strings.HasPrefix(browseID, "OLAK") {
		return MusicAlbum
	}
	if strings.HasPrefix(browseID, "VL") || strings.HasPrefix(browseID, "PL") ||
		strings.HasPrefix(browseID, "RDCLAK") || strings.HasPrefix(browseID, "RDEM") {
		return MusicPlaylist
	}

	// Check the first word of the subtitle (before " • ")
	first := strings.ToLower(subtitle)
	if i := strings.Index(first, " • "); i >= 0 {
		first = first[:i]
	}
	first = strings.TrimSpace(first)
	switch first {
	case "artist":
		return MusicArtist
	case "video":
		return MusicVideo
	case "playlist":
		return MusicPlaylist
	case "album", "single":
		return MusicAlbum
	case "ep":
		return MusicAlbum
	}

	return MusicSong
}
