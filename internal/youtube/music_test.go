package youtube

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestParseMusicSearchResponse(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_music_search_response.json")
	result, err := parseMusicSearchResponse(raw)
	if err != nil {
		t.Fatalf("parseMusicSearchResponse: %v", err)
	}

	t.Run("TopResult", func(t *testing.T) {
		if result.TopResult == nil {
			t.Fatal("expected top result, got nil")
		}
		assertEqual(t, "TopResult.Title", result.TopResult.Title, "Fake Artist")
		assertEqual(t, "TopResult.BrowseID", result.TopResult.BrowseID, "UCfake_music_artist")
		if result.TopResult.Type != MusicArtist {
			t.Errorf("TopResult.Type = %q, want %q", result.TopResult.Type, MusicArtist)
		}
		if len(result.TopResult.Thumbnails) == 0 {
			t.Error("expected top result thumbnails")
		}
	})

	t.Run("Shelves", func(t *testing.T) {
		if len(result.Shelves) != 2 {
			t.Fatalf("expected 2 shelves, got %d", len(result.Shelves))
		}
		assertEqual(t, "Shelves[0].Title", result.Shelves[0].Title, "Songs")
		assertEqual(t, "Shelves[1].Title", result.Shelves[1].Title, "Albums")
	})

	t.Run("Songs", func(t *testing.T) {
		songs := result.Shelves[0].Items
		if len(songs) != 2 {
			t.Fatalf("expected 2 songs, got %d", len(songs))
		}
		assertEqual(t, "Songs[0].Title", songs[0].Title, "Fake Song One")
		assertEqual(t, "Songs[0].VideoID", songs[0].VideoID, "fake_song_vid_001")
		if songs[0].Type != MusicSong {
			t.Errorf("Songs[0].Type = %q, want %q", songs[0].Type, MusicSong)
		}
		assertEqual(t, "Songs[1].Title", songs[1].Title, "Fake Song Two")
		assertEqual(t, "Songs[1].VideoID", songs[1].VideoID, "fake_song_vid_002")
	})

	t.Run("Albums", func(t *testing.T) {
		albums := result.Shelves[1].Items
		if len(albums) != 1 {
			t.Fatalf("expected 1 album, got %d", len(albums))
		}
		assertEqual(t, "Albums[0].Title", albums[0].Title, "Fake Album")
		assertEqual(t, "Albums[0].BrowseID", albums[0].BrowseID, "MPREfake_album_001")
		if albums[0].Type != MusicAlbum {
			t.Errorf("Albums[0].Type = %q, want %q", albums[0].Type, MusicAlbum)
		}
	})
}

func TestParseHomeResponse(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_music_home_response.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatalf("toGJSON: %v", err)
	}

	// Reuse the same parsing logic as GetHome
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
				return true
			})
			if len(items) > 0 {
				shelves = append(shelves, MusicShelf{Title: title, Items: items})
			}
			return true
		})
		return true
	})

	if len(shelves) != 2 {
		t.Fatalf("expected 2 shelves, got %d", len(shelves))
	}

	t.Run("ListenAgain", func(t *testing.T) {
		assertEqual(t, "Title", shelves[0].Title, "Listen again")
		if len(shelves[0].Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(shelves[0].Items))
		}
		assertEqual(t, "Items[0].Title", shelves[0].Items[0].Title, "Fake Playlist One")
		assertEqual(t, "Items[0].BrowseID", shelves[0].Items[0].BrowseID, "VLfake_playlist_001")
		if shelves[0].Items[0].Type != MusicPlaylist {
			t.Errorf("Items[0].Type = %q, want %q", shelves[0].Items[0].Type, MusicPlaylist)
		}
		assertEqual(t, "Items[1].Title", shelves[0].Items[1].Title, "Fake Album Home")
		if shelves[0].Items[1].Type != MusicAlbum {
			t.Errorf("Items[1].Type = %q, want %q", shelves[0].Items[1].Type, MusicAlbum)
		}
	})

	t.Run("Recommended", func(t *testing.T) {
		assertEqual(t, "Title", shelves[1].Title, "Recommended")
		if len(shelves[1].Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(shelves[1].Items))
		}
		assertEqual(t, "Items[0].VideoID", shelves[1].Items[0].VideoID, "fake_rec_vid")
	})
}

func TestParseLibrarySection(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_music_library_response.json")
	result, err := parseLibrarySection(raw)
	if err != nil {
		t.Fatalf("parseLibrarySection: %v", err)
	}

	t.Run("FiltersUIItems", func(t *testing.T) {
		// "New playlist" has no browseID/videoID and should be filtered
		for _, item := range result.Items {
			if item.Title == "New playlist" {
				t.Error("'New playlist' UI item should have been filtered out")
			}
		}
	})

	t.Run("Items", func(t *testing.T) {
		if len(result.Items) != 2 {
			t.Fatalf("expected 2 items (after filtering), got %d", len(result.Items))
		}
		assertEqual(t, "Items[0].Title", result.Items[0].Title, "Fake Library Playlist")
		assertEqual(t, "Items[0].BrowseID", result.Items[0].BrowseID, "VLfake_lib_pl_001")
		assertEqual(t, "Items[1].Title", result.Items[1].Title, "Liked Music")
		assertEqual(t, "Items[1].BrowseID", result.Items[1].BrowseID, "VLfake_liked_music")
	})

	t.Run("Continuation", func(t *testing.T) {
		assertEqual(t, "Continuation", result.Continuation, "fake_library_grid_continuation")
	})
}

func TestParseLibraryContinuation(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_music_library_continuation.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatalf("toGJSON: %v", err)
	}

	// Parse grid continuation (same logic as GetLibraryContinuation)
	result := &LibrarySectionResult{}
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
	ct := data.Get("continuationContents.gridContinuation.continuations.0.nextContinuationData.continuation").String()
	if ct != "" {
		result.Continuation = ct
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	assertEqual(t, "Items[0].Title", result.Items[0].Title, "Continued Playlist")
	assertEqual(t, "Items[0].BrowseID", result.Items[0].BrowseID, "VLfake_cont_pl_001")
	assertEqual(t, "Continuation", result.Continuation, "fake_next_grid_continuation")
}

func TestParseArtistPage(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_music_artist_response.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatalf("toGJSON: %v", err)
	}

	// Parse header
	name := data.Get("header.musicImmersiveHeaderRenderer.title.runs.0.text").String()
	assertEqual(t, "Name", name, "Fake Music Artist")

	// Parse shelves (same logic as GetArtist)
	var shelves []MusicShelf
	tabs := getMusicBrowseTabs(data)
	tabs.ForEach(func(_, tab gjson.Result) bool {
		sections := tab.Get("tabRenderer.content.sectionListRenderer.contents")
		sections.ForEach(func(_, section gjson.Result) bool {
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
				shelves = append(shelves, ms)
			}

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
					return true
				})
				ms := MusicShelf{Title: title, Items: items}
				mcb := hdr.Get("moreContentButton.buttonRenderer.navigationEndpoint.browseEndpoint")
				if mcb.Exists() {
					ms.MoreBrowseID = mcb.Get("browseId").String()
					ms.MoreParams = mcb.Get("params").String()
				}
				shelves = append(shelves, ms)
			}
			return true
		})
		return true
	})

	if len(shelves) != 2 {
		t.Fatalf("expected 2 shelves, got %d", len(shelves))
	}

	t.Run("Songs", func(t *testing.T) {
		s := shelves[0]
		assertEqual(t, "Title", s.Title, "Songs")
		if len(s.Items) != 1 {
			t.Fatalf("expected 1 song, got %d", len(s.Items))
		}
		assertEqual(t, "Items[0].Title", s.Items[0].Title, "Artist Song One")
		assertEqual(t, "Items[0].VideoID", s.Items[0].VideoID, "fake_artist_song_001")
		assertEqual(t, "MoreBrowseID", s.MoreBrowseID, "UCfake_music_artist")
		assertEqual(t, "MoreParams", s.MoreParams, "fake_songs_params")
	})

	t.Run("Albums", func(t *testing.T) {
		s := shelves[1]
		assertEqual(t, "Title", s.Title, "Albums")
		if len(s.Items) != 1 {
			t.Fatalf("expected 1 album, got %d", len(s.Items))
		}
		assertEqual(t, "Items[0].Title", s.Items[0].Title, "Fake Album One")
		assertEqual(t, "Items[0].BrowseID", s.Items[0].BrowseID, "MPREfake_alb_001")
		assertEqual(t, "MoreBrowseID", s.MoreBrowseID, "UCfake_music_artist")
		assertEqual(t, "MoreParams", s.MoreParams, "fake_albums_params")
	})
}

func TestParseAlbumPage(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_music_album_response.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatalf("toGJSON: %v", err)
	}

	// Parse header
	header := data.Get("header.musicDetailHeaderRenderer")
	title := header.Get("title.runs.0.text").String()
	assertEqual(t, "Title", title, "Fake Album Title")

	// Parse subtitle for artist/year
	var artist, year string
	header.Get("subtitle.runs").ForEach(func(_, run gjson.Result) bool {
		text := run.Get("text").String()
		if artist == "" && text != "Album" && text != " • " && text != "Playlist" {
			artist = text
		}
		if len(text) == 4 && text[0] >= '1' && text[0] <= '2' {
			year = text
		}
		return true
	})
	assertEqual(t, "Artist", artist, "Fake Album Artist")
	assertEqual(t, "Year", year, "2024")

	// Parse tracks from secondaryContents
	sections := data.Get("contents.twoColumnBrowseResultsRenderer.secondaryContents.sectionListRenderer.contents")
	var tracks []MusicItem
	var playlistID string
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
			t := mrlir.Get("flexColumns.0.musicResponsiveListItemFlexColumnRenderer.text.runs.0.text").String()
			vid := mrlir.Get("overlay.musicItemThumbnailOverlayRenderer.content.musicPlayButtonRenderer.playNavigationEndpoint.watchEndpoint.videoId").String()
			plid := mrlir.Get("overlay.musicItemThumbnailOverlayRenderer.content.musicPlayButtonRenderer.playNavigationEndpoint.watchEndpoint.playlistId").String()
			if playlistID == "" && plid != "" {
				playlistID = plid
			}
			dur := mrlir.Get("fixedColumns.0.musicResponsiveListItemFixedColumnRenderer.text.runs.0.text").String()
			tracks = append(tracks, MusicItem{Type: MusicSong, Title: t, VideoID: vid, Subtitle: dur})
			return true
		})
		return false
	})

	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}
	assertEqual(t, "Tracks[0].Title", tracks[0].Title, "Track One")
	assertEqual(t, "Tracks[0].VideoID", tracks[0].VideoID, "fake_track_001")
	assertEqual(t, "Tracks[0].Subtitle", tracks[0].Subtitle, "3:45")
	assertEqual(t, "Tracks[1].Title", tracks[1].Title, "Track Two")
	assertEqual(t, "Tracks[1].VideoID", tracks[1].VideoID, "fake_track_002")
	assertEqual(t, "PlaylistID", playlistID, "OLAKfake_playlist_id")
}

func TestParsePlaylistAsTracks(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_music_playlist_response.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatalf("toGJSON: %v", err)
	}

	// Verify header filters "Playlist" from artist
	header := data.Get("header.musicDetailHeaderRenderer")
	var artist string
	header.Get("subtitle.runs").ForEach(func(_, run gjson.Result) bool {
		text := run.Get("text").String()
		if artist == "" && text != "Album" && text != " • " && text != "EP" && text != "Single" && text != "Playlist" {
			artist = text
		}
		return true
	})
	assertEqual(t, "Artist", artist, "FakeUser")

	// Verify tracks are found via singleColumnBrowseResultsRenderer + musicPlaylistShelfRenderer
	var tracks []MusicItem
	tabs := getMusicBrowseTabs(data)
	tabs.ForEach(func(_, tab gjson.Result) bool {
		tab.Get("tabRenderer.content.sectionListRenderer.contents").ForEach(func(_, section gjson.Result) bool {
			if shelf := section.Get("musicPlaylistShelfRenderer"); shelf.Exists() {
				shelf.Get("contents").ForEach(func(_, entry gjson.Result) bool {
					mrlir := entry.Get("musicResponsiveListItemRenderer")
					if mrlir.Exists() {
						t := mrlir.Get("flexColumns.0.musicResponsiveListItemFlexColumnRenderer.text.runs.0.text").String()
						vid := mrlir.Get("overlay.musicItemThumbnailOverlayRenderer.content.musicPlayButtonRenderer.playNavigationEndpoint.watchEndpoint.videoId").String()
						tracks = append(tracks, MusicItem{Type: MusicSong, Title: t, VideoID: vid})
					}
					return true
				})
			}
			return true
		})
		return true
	})

	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	assertEqual(t, "Tracks[0].Title", tracks[0].Title, "Playlist Track One")
	assertEqual(t, "Tracks[0].VideoID", tracks[0].VideoID, "fake_pl_track_001")
}

func TestDetectMusicItemType(t *testing.T) {
	tests := []struct {
		name     string
		subtitle string
		browseID string
		want     MusicItemType
	}{
		{"Artist from subtitle", "Artist • 1M subscribers", "", MusicArtist},
		{"Video from subtitle", "Video • Artist", "", MusicVideo},
		{"UC prefix", "100K subscribers", "UCsomeartist", MusicArtist},
		{"MPRE prefix", "2023", "MPREsomealbum", MusicAlbum},
		{"OLAK prefix", "2024", "OLAKsomealbum", MusicAlbum},
		{"VL prefix", "50 songs", "VLsomeplaylist", MusicPlaylist},
		{"PL prefix", "20 songs", "PLuserplaylist", MusicPlaylist},
		{"RDCLAK prefix", "", "RDCLAKsomething", MusicPlaylist},
		{"RDEM prefix", "", "RDEMsomething", MusicPlaylist},
		{"Playlist from subtitle", "playlist • user", "", MusicPlaylist},
		{"Album from subtitle", "album • artist", "", MusicAlbum},
		{"Single from subtitle", "single • artist", "", MusicAlbum},
		{"EP exact match", "EP • artist", "", MusicAlbum},
		{"EP not greedy", "September • artist", "", MusicSong},
		{"Default to song", "some subtitle", "", MusicSong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectMusicItemType(tt.subtitle, tt.browseID)
			if got != tt.want {
				t.Errorf("detectMusicItemType(%q, %q) = %q, want %q", tt.subtitle, tt.browseID, got, tt.want)
			}
		})
	}
}

func TestParseMusicListItem_BrowseIDFallback(t *testing.T) {
	// Library artist items have browseID in the title run's navigation, not the top-level
	raw := loadFixture(t, "testdata/fake_music_search_response.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatalf("toGJSON: %v", err)
	}

	// Get the album item which has browseID in navigationEndpoint
	albumEntry := data.Get("contents.tabbedSearchResultsRenderer.tabs.0.tabRenderer.content.sectionListRenderer.contents.2.musicShelfRenderer.contents.0.musicResponsiveListItemRenderer")
	if !albumEntry.Exists() {
		t.Fatal("could not find album entry in fixture")
	}

	item := parseMusicListItem(albumEntry)
	assertEqual(t, "Title", item.Title, "Fake Album")
	assertEqual(t, "BrowseID", item.BrowseID, "MPREfake_album_001")
	if item.Type != MusicAlbum {
		t.Errorf("Type = %q, want %q", item.Type, MusicAlbum)
	}
}

func TestParseMusicTwoRowItem_VideoIDFallbacks(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_music_home_response.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatalf("toGJSON: %v", err)
	}

	// Recommended song has videoID in overlay path
	recItem := data.Get("contents.singleColumnBrowseResultsRenderer.tabs.0.tabRenderer.content.sectionListRenderer.contents.1.musicCarouselShelfRenderer.contents.0.musicTwoRowItemRenderer")
	if !recItem.Exists() {
		t.Fatal("could not find recommended item")
	}
	item := parseMusicTwoRowItem(recItem)
	assertEqual(t, "VideoID", item.VideoID, "fake_rec_vid")

	// Playlist item has browseID via navigationEndpoint
	plItem := data.Get("contents.singleColumnBrowseResultsRenderer.tabs.0.tabRenderer.content.sectionListRenderer.contents.0.musicCarouselShelfRenderer.contents.0.musicTwoRowItemRenderer")
	if !plItem.Exists() {
		t.Fatal("could not find playlist item")
	}
	pl := parseMusicTwoRowItem(plItem)
	assertEqual(t, "BrowseID", pl.BrowseID, "VLfake_playlist_001")
	if pl.Type != MusicPlaylist {
		t.Errorf("Type = %q, want %q", pl.Type, MusicPlaylist)
	}
}

func TestGetMusicBrowseTabs(t *testing.T) {
	t.Run("SingleColumn", func(t *testing.T) {
		raw := loadFixture(t, "testdata/fake_music_home_response.json")
		data, err := toGJSON(raw)
		if err != nil {
			t.Fatalf("toGJSON: %v", err)
		}
		tabs := getMusicBrowseTabs(data)
		if !tabs.Exists() {
			t.Error("expected tabs from singleColumnBrowseResultsRenderer")
		}
	})

	t.Run("TwoColumn", func(t *testing.T) {
		raw := loadFixture(t, "testdata/fake_music_album_response.json")
		data, err := toGJSON(raw)
		if err != nil {
			t.Fatalf("toGJSON: %v", err)
		}
		// Album uses twoColumnBrowseResultsRenderer but has no tabs at top level
		// (tracks are in secondaryContents). This tests that the helper
		// doesn't panic on missing paths.
		_ = getMusicBrowseTabs(data)
	})
}

