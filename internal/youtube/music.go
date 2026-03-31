package youtube

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	innertubego "github.com/nezbut/innertube-go"
	"github.com/tidwall/gjson"
)

// MusicClient provides YouTube Music API access via InnerTube WEB_REMIX client.
type MusicClient struct {
	it *innertubego.InnerTube
}

// NewMusicClient creates a new YouTube Music client.
// Pass a custom httpClient with a cookie jar for authenticated requests.
func NewMusicClient(httpClient *http.Client) (*MusicClient, error) {
	it, err := innertubego.NewInnerTube(httpClient, "WEB_REMIX", "1.20230724.00.00", "", "", "", nil, true)
	if err != nil {
		return nil, fmt.Errorf("music innertube init: %w", err)
	}
	return &MusicClient{it: it}, nil
}

// Search searches YouTube Music for the given query.
func (c *MusicClient) Search(ctx context.Context, query string) (*MusicSearchResult, error) {
	raw, err := c.it.Search(ctx, &query, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("music search: %w", err)
	}
	return parseMusicSearchResponse(raw)
}

// Browse fetches a YouTube Music browse page (home, album, artist, playlist).
func (c *MusicClient) Browse(ctx context.Context, browseID string) (map[string]interface{}, error) {
	raw, err := c.it.Browse(ctx, &browseID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("music browse: %w", err)
	}
	return raw, nil
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
	tabs := data.Get("contents.singleColumnBrowseResultsRenderer.tabs")
	if !tabs.Exists() {
		tabs = data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	}
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
		header.Get("subtitle.runs").ForEach(func(_, run gjson.Result) bool {
			text := run.Get("text").String()
			if page.Artist == "" && text != "Album" && text != " • " && text != "EP" && text != "Single" {
				page.Artist = text
			}
			// Year is typically the last numeric part
			if len(text) == 4 && text[0] >= '1' && text[0] <= '2' {
				page.Year = text
			}
			return true
		})
	}

	// Tracks from secondaryContents
	sections := data.Get("contents.twoColumnBrowseResultsRenderer.secondaryContents.sectionListRenderer.contents")
	sections.ForEach(func(_, section gjson.Result) bool {
		shelf := section.Get("musicShelfRenderer")
		if !shelf.Exists() {
			return true
		}
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

			// Duration from fixedColumns
			dur := mrlir.Get("fixedColumns.0.musicResponsiveListItemFixedColumnRenderer.text.runs.0.text").String()

			page.Tracks = append(page.Tracks, MusicItem{
				Type:        MusicSong,
				Title:       title,
				VideoID:     vid,
				Subtitle:    dur,
			})
			return true
		})
		return false
	})

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

	mtrir.Get("thumbnailRenderer.musicThumbnailRenderer.thumbnail.thumbnails").ForEach(func(_, t gjson.Result) bool {
		item.Thumbnails = append(item.Thumbnails, Thumbnail{
			URL:    t.Get("url").String(),
			Width:  int(t.Get("width").Int()),
			Height: int(t.Get("height").Int()),
		})
		return true
	})

	return item
}

// BrowseMore fetches a "See all" page (e.g., all albums for an artist).
// Returns the items from gridRenderer or musicShelfRenderer.
func (c *MusicClient) BrowseMore(ctx context.Context, browseID, params string) ([]MusicItem, error) {
	raw, err := c.it.Browse(ctx, &browseID, &params, nil)
	if err != nil {
		return nil, fmt.Errorf("music browse more: %w", err)
	}
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	var items []MusicItem
	tabs := data.Get("contents.singleColumnBrowseResultsRenderer.tabs")
	if !tabs.Exists() {
		tabs = data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	}
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

	sections := data.Get("contents.tabbedSearchResultsRenderer.tabs.0.tabRenderer.content.sectionListRenderer.contents")
	sections.ForEach(func(_, section gjson.Result) bool {
		// Top result card (artist/album)
		card := section.Get("musicCardShelfRenderer")
		if card.Exists() {
			item := parseMusicCard(card)
			result.TopResult = &item
			return true
		}

		// Shelf of results
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
		return true
	})

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

	card.Get("thumbnail.musicThumbnailRenderer.thumbnail.thumbnails").ForEach(func(_, t gjson.Result) bool {
		item.Thumbnails = append(item.Thumbnails, Thumbnail{
			URL:    t.Get("url").String(),
			Width:  int(t.Get("width").Int()),
			Height: int(t.Get("height").Int()),
		})
		return true
	})

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

	// Detect type from first word of subtitle
	itemType := MusicSong
	if len(subtitleParts) > 0 {
		first := strings.TrimSpace(subtitleParts[0])
		switch first {
		case "Album":
			itemType = MusicAlbum
		case "Artist":
			itemType = MusicArtist
		case "Playlist":
			itemType = MusicPlaylist
		case "Video":
			itemType = MusicVideo
		case "Song":
			itemType = MusicSong
		}
	}

	// Get videoId (for playable items)
	videoID := mrlir.Get("overlay.musicItemThumbnailOverlayRenderer.content.musicPlayButtonRenderer.playNavigationEndpoint.watchEndpoint.videoId").String()

	// Get browseId (for browsable items)
	browseID := mrlir.Get("navigationEndpoint.browseEndpoint.browseId").String()

	item := MusicItem{
		Type:     itemType,
		Title:    title,
		Subtitle: subtitle,
		VideoID:  videoID,
		BrowseID: browseID,
	}

	mrlir.Get("thumbnail.musicThumbnailRenderer.thumbnail.thumbnails").ForEach(func(_, t gjson.Result) bool {
		item.Thumbnails = append(item.Thumbnails, Thumbnail{
			URL:    t.Get("url").String(),
			Width:  int(t.Get("width").Int()),
			Height: int(t.Get("height").Int()),
		})
		return true
	})

	return item
}

func detectMusicItemType(subtitle, browseID string) MusicItemType {
	lower := strings.ToLower(subtitle)
	if strings.Contains(lower, "artist") {
		return MusicArtist
	}
	if strings.Contains(lower, "video") {
		return MusicVideo
	}
	if strings.HasPrefix(browseID, "MPRE") {
		return MusicAlbum
	}
	if strings.HasPrefix(browseID, "VL") {
		return MusicPlaylist
	}
	return MusicSong
}
