package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	innertubego "github.com/nezbut/innertube-go"
	"github.com/tidwall/gjson"
)

const searchFilterVideosOnly = "EgIQAQ%3D%3D"

// Shared user-agent for all InnerTube clients.
const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// WEB client static config. Version and API key are scraped at startup
// (see scrape.go) and read from clientParams.
const (
	webClientName = "WEB"
	webClientID   = 1
	webReferer    = "https://www.youtube.com/"
)

// VideoURL returns the canonical watch URL for a video ID.
func VideoURL(id string) string {
	return "https://www.youtube.com/watch?v=" + id
}

// PlaylistURL returns the canonical playlist URL for a playlist ID.
func PlaylistURL(id string) string {
	return "https://www.youtube.com/playlist?list=" + id
}

// MusicPlaylistURL returns a YouTube Music playlist URL for a playlist ID.
func MusicPlaylistURL(id string) string {
	return "https://music.youtube.com/playlist?list=" + id
}

// URLKind identifies the type of YouTube URL.
type URLKind int

const (
	URLUnknown URLKind = iota
	URLVideo
	URLPlaylist
	URLChannel
)

// ParsedURL holds the detected type and ID from a YouTube URL.
type ParsedURL struct {
	Kind URLKind
	ID   string
}

// ParseYouTubeURL detects the type and extracts the ID from a YouTube URL.
// If the input has no URL structure, it is treated as a raw video ID.
func ParseYouTubeURL(raw string) ParsedURL {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ParsedURL{Kind: URLUnknown}
	}

	// No scheme or dots → raw video ID (YouTube IDs are 11 chars, alphanumeric + -_)
	if !strings.Contains(raw, ".") && !strings.Contains(raw, "/") {
		if isValidVideoID(raw) {
			return ParsedURL{Kind: URLVideo, ID: raw}
		}
		return ParsedURL{Kind: URLUnknown}
	}

	// Ensure scheme for url.Parse
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return ParsedURL{Kind: URLUnknown}
	}

	host := strings.TrimPrefix(u.Hostname(), "www.")
	host = strings.TrimPrefix(host, "m.")
	host = strings.TrimPrefix(host, "music.")
	path := strings.TrimSuffix(u.Path, "/")

	switch host {
	case "youtube.com":
		// /watch?v=ID
		if path == "/watch" {
			if v := u.Query().Get("v"); v != "" {
				return ParsedURL{Kind: URLVideo, ID: v}
			}
		}
		// /playlist?list=ID
		if path == "/playlist" {
			if list := u.Query().Get("list"); list != "" {
				return ParsedURL{Kind: URLPlaylist, ID: list}
			}
		}
		// /channel/ID
		if strings.HasPrefix(path, "/channel/") {
			id := strings.TrimPrefix(path, "/channel/")
			if id != "" {
				return ParsedURL{Kind: URLChannel, ID: id}
			}
		}
		// /@handle
		if strings.HasPrefix(path, "/@") {
			handle := strings.TrimPrefix(path, "/")
			if handle != "" {
				return ParsedURL{Kind: URLChannel, ID: handle}
			}
		}
	case "youtu.be":
		id := strings.TrimPrefix(path, "/")
		if isValidVideoID(id) {
			return ParsedURL{Kind: URLVideo, ID: id}
		}
	default:
		// Fallback for Invidious and other YouTube-compatible frontends (e.g. yewtu.be).
		if path == "/watch" {
			if v := u.Query().Get("v"); v != "" {
				return ParsedURL{Kind: URLVideo, ID: v}
			}
		}
		id := strings.TrimPrefix(path, "/")
		if isValidVideoID(id) {
			return ParsedURL{Kind: URLVideo, ID: id}
		}
	}

	return ParsedURL{Kind: URLUnknown}
}

var videoIDRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

func isValidVideoID(s string) bool {
	return videoIDRegex.MatchString(s)
}

// InnerTubeClient implements Client using YouTube's InnerTube API.
type InnerTubeClient struct {
	it            *innertubego.InnerTube
	authenticated bool
}

func newInnerTubeWEB(httpClient *http.Client) *innertubego.InnerTube {
	return &innertubego.InnerTube{
		Adaptor: innertubego.NewInnerTubeAdaptor(innertubego.ClientContext{
			ClientName:    webClientName,
			ClientVersion: clientParams.web.ClientVersion,
			ClientID:      webClientID,
			APIKey:        clientParams.web.APIKey,
			UserAgent:     defaultUserAgent,
			Referer:       webReferer,
		}, httpClient),
	}
}

// NewInnerTubeClient creates a new InnerTube-backed client.
// Pass a custom httpClient with a cookie jar for authenticated requests.
func NewInnerTubeClient(httpClient *http.Client) (*InnerTubeClient, error) {
	it := newInnerTubeWEB(httpClient)
	return &InnerTubeClient{it: it, authenticated: httpClient != nil && httpClient.Jar != nil}, nil
}

func (c *InnerTubeClient) Search(ctx context.Context, query string, pageToken string) (*Page[Video], error) {
	params := searchFilterVideosOnly
	var cont *string
	if pageToken != "" {
		cont = &pageToken
	}

	raw, err := c.it.Search(ctx, &query, &params, cont)
	if err != nil {
		return nil, fmt.Errorf("innertube search: %w", err)
	}

	return parseSearchResponse(raw)
}

func (c *InnerTubeClient) GetVideo(ctx context.Context, id string) (*Video, error) {
	// Fetch Player and Next endpoints in parallel
	type playerResult struct {
		raw map[string]interface{}
		err error
	}
	type nextResult struct {
		raw map[string]interface{}
		err error
	}

	playerCh := make(chan playerResult, 1)
	nextCh := make(chan nextResult, 1)

	go func() {
		raw, err := c.it.Player(ctx, id)
		playerCh <- playerResult{raw, err}
	}()
	go func() {
		raw, err := c.it.Next(ctx, &id, nil, nil, nil, nil)
		nextCh <- nextResult{raw, err}
	}()

	pr := <-playerCh
	if pr.err != nil {
		return nil, fmt.Errorf("innertube player: %w", pr.err)
	}

	data, err := toGJSON(pr.raw)
	if err != nil {
		return nil, err
	}
	vd := data.Get("videoDetails")
	if !vd.Exists() {
		return nil, fmt.Errorf("no video details for %s", id)
	}

	v := &Video{
		ID:          vd.Get("videoId").String(),
		Title:       vd.Get("title").String(),
		ChannelName: vd.Get("author").String(),
		ChannelID:   vd.Get("channelId").String(),
		Description: vd.Get("shortDescription").String(),
		DurationStr: formatSeconds(vd.Get("lengthSeconds").String()),
		ViewCount:   vd.Get("viewCount").String(),
		URL:         VideoURL(vd.Get("videoId").String()),
	}

	v.Thumbnails = parseThumbnails(vd.Get("thumbnail.thumbnails"))

	// Enrich with Next endpoint data (formatted views, likes, date, subscriber count)
	nr := <-nextCh
	if nr.err == nil {
		enrichVideo(v, nr.raw)
		v.CommentsToken = extractCommentsToken(nr.raw)
	}

	return v, nil
}

func enrichVideo(v *Video, raw map[string]interface{}) {
	data, err := toGJSON(raw)
	if err != nil {
		return
	}

	sections := data.Get("contents.twoColumnWatchNextResults.results.results.contents")
	sections.ForEach(func(_, item gjson.Result) bool {
		// Primary info: formatted views, date, likes
		vpir := item.Get("videoPrimaryInfoRenderer")
		if vpir.Exists() {
			if fv := vpir.Get("viewCount.videoViewCountRenderer.viewCount.simpleText"); fv.Exists() {
				v.ViewCount = fv.String()
			}
			if dt := vpir.Get("dateText.simpleText"); dt.Exists() {
				v.PublishedAt = dt.String()
			}
			likes := vpir.Get("videoActions.menuRenderer.topLevelButtons.0.segmentedLikeDislikeButtonViewModel.likeButtonViewModel.likeButtonViewModel.toggleButtonViewModel.toggleButtonViewModel.defaultButtonViewModel.buttonViewModel.title")
			if likes.Exists() {
				v.LikeCount = likes.String()
			}
		}

		// Secondary info: subscriber count, full description
		vsir := item.Get("videoSecondaryInfoRenderer")
		if vsir.Exists() {
			if sc := vsir.Get("owner.videoOwnerRenderer.subscriberCountText.simpleText"); sc.Exists() {
				v.SubscriberCount = sc.String()
			}
			if desc := vsir.Get("attributedDescription.content"); desc.Exists() {
				v.Description = desc.String()
			}
		}
		return true
	})
}

func (c *InnerTubeClient) GetComments(ctx context.Context, videoID string, pageToken string) (*Page[Comment], error) {
	if pageToken == "" {
		return &Page[Comment]{}, nil
	}
	raw, err := c.it.Next(ctx, nil, nil, nil, nil, &pageToken)
	if err != nil {
		return nil, fmt.Errorf("innertube comments: %w", err)
	}
	return parseCommentsResponse(raw)
}

func (c *InnerTubeClient) GetReplies(ctx context.Context, commentID string, pageToken string) (*Page[Comment], error) {
	if pageToken == "" {
		return &Page[Comment]{}, nil
	}
	raw, err := c.it.Next(ctx, nil, nil, nil, nil, &pageToken)
	if err != nil {
		return nil, fmt.Errorf("innertube replies: %w", err)
	}
	return parseCommentsResponse(raw)
}

func extractCommentsToken(raw map[string]interface{}) string {
	data, err := toGJSON(raw)
	if err != nil {
		return ""
	}
	var token string
	data.Get("engagementPanels").ForEach(func(_, panel gjson.Result) bool {
		ep := panel.Get("engagementPanelSectionListRenderer")
		if ep.Get("panelIdentifier").String() == "engagement-panel-comments-section" {
			t := ep.Get("content.sectionListRenderer.contents.0.itemSectionRenderer.contents.0.continuationItemRenderer.continuationEndpoint.continuationCommand.token")
			if t.Exists() {
				token = t.String()
			}
		}
		return true
	})
	return token
}

func parseCommentsResponse(raw map[string]interface{}) (*Page[Comment], error) {
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	// Build lookup map from entity mutations
	commentEntities := map[string]gjson.Result{}
	data.Get("frameworkUpdates.entityBatchUpdate.mutations").ForEach(func(_, m gjson.Result) bool {
		ce := m.Get("payload.commentEntityPayload")
		if ce.Exists() {
			key := ce.Get("key").String()
			if key != "" {
				commentEntities[key] = ce
			}
		}
		return true
	})

	var comments []Comment
	var nextToken string

	// Extract comment thread keys and reply tokens from onResponseReceivedEndpoints
	data.Get("onResponseReceivedEndpoints").ForEach(func(_, ep gjson.Result) bool {
		items := ep.Get("reloadContinuationItemsCommand.continuationItems")
		if !items.Exists() {
			items = ep.Get("appendContinuationItemsAction.continuationItems")
		}
		if !items.Exists() {
			return true
		}
		items.ForEach(func(_, item gjson.Result) bool {
			// Top-level comment thread (has replies container)
			ctr := item.Get("commentThreadRenderer")
			if ctr.Exists() {
				commentKey := ctr.Get("commentViewModel.commentViewModel.commentKey").String()
				if ce, ok := commentEntities[commentKey]; ok {
					c := parseCommentEntity(ce)
					replyCi := ctr.Get("replies.commentRepliesRenderer.contents.0.continuationItemRenderer")
					if rt := extractContinuationToken(replyCi); rt != "" {
						c.ReplyToken = rt
					}
					comments = append(comments, c)
				}
				return true
			}
			// Reply or standalone comment (commentViewModel directly)
			cvm := item.Get("commentViewModel")
			if cvm.Exists() {
				commentKey := cvm.Get("commentKey").String()
				if ce, ok := commentEntities[commentKey]; ok {
					comments = append(comments, parseCommentEntity(ce))
				}
				return true
			}
			ci := item.Get("continuationItemRenderer")
			if ci.Exists() {
				if t := extractContinuationToken(ci); t != "" {
					nextToken = t
				}
			}
			return true
		})
		return true
	})

	return &Page[Comment]{
		Items:     comments,
		NextToken: nextToken,
		HasMore:   nextToken != "",
	}, nil
}

func extractContinuationToken(ci gjson.Result) string {
	token := ci.Get("continuationEndpoint.continuationCommand.token")
	if !token.Exists() {
		token = ci.Get("button.buttonRenderer.command.continuationCommand.token")
	}
	return token.String()
}

func parseCommentEntity(ce gjson.Result) Comment {
	props := ce.Get("properties")
	author := ce.Get("author")
	toolbar := ce.Get("toolbar")

	return Comment{
		ID:          props.Get("commentId").String(),
		AuthorName:  author.Get("displayName").String(),
		AuthorID:    author.Get("channelId").String(),
		Content:     props.Get("content.content").String(),
		LikeCount:   toolbar.Get("likeCountNotliked").String(),
		ReplyCount:  toolbar.Get("replyCount").Int(),
		PublishedAt: props.Get("publishedTime").String(),
		IsOwner:     author.Get("isCreator").Bool(),
	}
}

func (c *InnerTubeClient) GetSubscriptions(ctx context.Context, pageToken string) (*Page[Channel], error) {
	if !c.authenticated {
		return nil, fmt.Errorf("authentication required for subscriptions")
	}

	browseID := "FEchannels"
	var cont *string
	if pageToken != "" {
		cont = &pageToken
	}
	raw, err := c.it.Browse(ctx, &browseID, nil, cont)
	if err != nil {
		return nil, fmt.Errorf("innertube browse channels: %w", err)
	}

	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	var channels []Channel
	var nextToken string

	// Initial response: tabs[].sectionListRenderer.contents[].itemSectionRenderer
	//   .contents[].shelfRenderer.content.expandedShelfContentsRenderer.items[].channelRenderer
	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		sections := tab.Get("tabRenderer.content.sectionListRenderer.contents")
		parseChannelSections(sections, &channels, &nextToken)
		return true
	})

	// Continuation response: onResponseReceivedActions[].appendContinuationItemsAction.continuationItems
	if len(channels) == 0 {
		data.Get("onResponseReceivedActions").ForEach(func(_, action gjson.Result) bool {
			items := action.Get("appendContinuationItemsAction.continuationItems")
			if items.Exists() {
				parseChannelSections(items, &channels, &nextToken)
			}
			return true
		})
	}

	return &Page[Channel]{
		Items:     channels,
		NextToken: nextToken,
		HasMore:   nextToken != "",
	}, nil
}

func parseChannelSections(sections gjson.Result, channels *[]Channel, nextToken *string) {
	sections.ForEach(func(_, section gjson.Result) bool {
		// Channel items inside shelves
		section.Get("itemSectionRenderer.contents").ForEach(func(_, content gjson.Result) bool {
			items := content.Get("shelfRenderer.content.expandedShelfContentsRenderer.items")
			items.ForEach(func(_, item gjson.Result) bool {
				cr := item.Get("channelRenderer")
				if !cr.Exists() {
					return true
				}
				subCount := cr.Get("subscriberCountText.simpleText").String()
				handle := ""
				// For non-Topic channels, subscriberCountText contains the handle
				// and the actual subscriber count is in videoCountText
				if len(subCount) > 0 && subCount[0] == '@' {
					handle = subCount
					subCount = cr.Get("videoCountText.simpleText").String()
				}
				ch := Channel{
					ID:              cr.Get("channelId").String(),
					Name:            cr.Get("title.simpleText").String(),
					Handle:          handle,
					SubscriberCount: subCount,
					URL:             "https://www.youtube.com/channel/" + cr.Get("channelId").String(),
				}
				ch.Thumbnails = parseThumbnails(cr.Get("thumbnail.thumbnails"))
				*channels = append(*channels, ch)
				return true
			})
			return true
		})

		// Continuation token
		token := section.Get("continuationItemRenderer.continuationEndpoint.continuationCommand.token")
		if token.Exists() {
			*nextToken = token.String()
		}
		return true
	})
}

func (c *InnerTubeClient) GetFeed(ctx context.Context, pageToken string) (*Page[Video], error) {
	if !c.authenticated {
		return nil, fmt.Errorf("authentication required for subscription feed")
	}

	browseID := "FEsubscriptions"
	var cont *string
	if pageToken != "" {
		cont = &pageToken
	}
	raw, err := c.it.Browse(ctx, &browseID, nil, cont)
	if err != nil {
		return nil, fmt.Errorf("innertube browse subscriptions: %w", err)
	}

	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	var videos []Video
	var nextToken string

	// Navigate to the selected tab's richGridRenderer (initial load)
	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		tr := tab.Get("tabRenderer")
		if !tr.Exists() || !tr.Get("selected").Bool() {
			return true
		}
		contents := tr.Get("content.richGridRenderer.contents")
		parseFeedItems(contents, &videos, &nextToken)
		return false // stop after selected tab
	})

	// Continuation response
	if len(videos) == 0 {
		data.Get("onResponseReceivedActions").ForEach(func(_, action gjson.Result) bool {
			items := action.Get("appendContinuationItemsAction.continuationItems")
			if items.Exists() {
				parseFeedItems(items, &videos, &nextToken)
			}
			return true
		})
	}

	return &Page[Video]{
		Items:     videos,
		NextToken: nextToken,
		HasMore:   nextToken != "",
	}, nil
}

// parseFeedItems parses video items from feed/channel richGridRenderer contents.
// Delegates to parseChannelVideoItems which handles all known video item formats.
func parseFeedItems(contents gjson.Result, videos *[]Video, nextToken *string) {
	parseChannelVideoItems(contents, videos, nextToken)
}

// Channel tab params (base64 protobuf filters).
const (
	channelVideosParams    = "EgZ2aWRlb3PyBgQKAjoA"
	channelPlaylistsParams = "EglwbGF5bGlzdHPyBgQKAkIA"
	channelPostsParams     = "EgVwb3N0c_IGBAoCSgA="
)

// browseChannelTab calls Browse for a channel tab. On initial load it sends
// the tab params; on continuation it sends the page token instead.
func (c *InnerTubeClient) browseChannelTab(ctx context.Context, channelID, params, pageToken string) (gjson.Result, error) {
	var cont, paramsPtr *string
	if pageToken != "" {
		cont = &pageToken
	} else {
		paramsPtr = &params
	}
	raw, err := c.it.Browse(ctx, &channelID, paramsPtr, cont)
	if err != nil {
		return gjson.Result{}, err
	}
	return toGJSON(raw)
}

func (c *InnerTubeClient) GetChannelVideos(ctx context.Context, channelID string, pageToken string) (*Page[Video], error) {
	data, err := c.browseChannelTab(ctx, channelID, channelVideosParams, pageToken)
	if err != nil {
		return nil, fmt.Errorf("innertube channel videos: %w", err)
	}

	var videos []Video
	var nextToken string

	// Initial response: selected tab → richGridRenderer.contents
	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		tr := tab.Get("tabRenderer")
		if !tr.Exists() || !tr.Get("selected").Bool() {
			return true
		}
		contents := tr.Get("content.richGridRenderer.contents")
		if contents.Exists() {
			parseChannelVideoItems(contents, &videos, &nextToken)
			return false
		}
		// Fallback: sectionListRenderer (some channels use this)
		sections := tr.Get("content.sectionListRenderer.contents")
		sections.ForEach(func(_, section gjson.Result) bool {
			items := section.Get("itemSectionRenderer.contents.0.gridRenderer.items")
			if items.Exists() {
				parseChannelVideoItems(items, &videos, &nextToken)
			}
			return true
		})
		return false
	})

	// Continuation response
	if len(videos) == 0 {
		data.Get("onResponseReceivedActions").ForEach(func(_, action gjson.Result) bool {
			items := action.Get("appendContinuationItemsAction.continuationItems")
			if items.Exists() {
				parseChannelVideoItems(items, &videos, &nextToken)
			}
			return true
		})
	}

	return &Page[Video]{
		Items:     videos,
		NextToken: nextToken,
		HasMore:   nextToken != "",
	}, nil
}

func parseChannelVideoItems(contents gjson.Result, videos *[]Video, nextToken *string) {
	contents.ForEach(func(_, item gjson.Result) bool {
		// richItemRenderer wrapping videoRenderer
		vr := item.Get("richItemRenderer.content.videoRenderer")
		if vr.Exists() {
			*videos = append(*videos, parseVideoRenderer(vr))
			return true
		}
		// richItemRenderer wrapping lockupViewModel
		lvm := item.Get("richItemRenderer.content.lockupViewModel")
		if lvm.Exists() {
			*videos = append(*videos, parseLockupViewModel(lvm))
			return true
		}
		// gridVideoRenderer (older layout)
		gvr := item.Get("gridVideoRenderer")
		if gvr.Exists() {
			*videos = append(*videos, parseGridVideoRenderer(gvr))
			return true
		}
		// Continuation token
		token := item.Get("continuationItemRenderer.continuationEndpoint.continuationCommand.token")
		if token.Exists() {
			*nextToken = token.String()
		}
		return true
	})
}

func parseGridVideoRenderer(gvr gjson.Result) Video {
	v := Video{
		ID:          gvr.Get("videoId").String(),
		Title:       gvr.Get("title.runs.0.text").String(),
		DurationStr: gvr.Get("thumbnailOverlays.0.thumbnailOverlayTimeStatusRenderer.text.simpleText").String(),
		ViewCount:   gvr.Get("viewCountText.simpleText").String(),
		PublishedAt: gvr.Get("publishedTimeText.simpleText").String(),
	}
	v.URL = VideoURL(v.ID)
	v.Thumbnails = parseThumbnails(gvr.Get("thumbnail.thumbnails"))
	return v
}

func (c *InnerTubeClient) GetChannelPlaylists(ctx context.Context, channelID string, pageToken string) (*Page[Playlist], error) {
	data, err := c.browseChannelTab(ctx, channelID, channelPlaylistsParams, pageToken)
	if err != nil {
		return nil, fmt.Errorf("innertube channel playlists: %w", err)
	}

	var playlists []Playlist
	var nextToken string

	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		tr := tab.Get("tabRenderer")
		if !tr.Exists() || !tr.Get("selected").Bool() {
			return true
		}
		sections := tr.Get("content.sectionListRenderer.contents")
		sections.ForEach(func(_, section gjson.Result) bool {
			items := section.Get("itemSectionRenderer.contents.0.gridRenderer.items")
			if items.Exists() {
				parseChannelPlaylistItems(items, &playlists, &nextToken)
			}
			return true
		})
		return false
	})

	// Continuation response
	if len(playlists) == 0 {
		data.Get("onResponseReceivedActions").ForEach(func(_, action gjson.Result) bool {
			items := action.Get("appendContinuationItemsAction.continuationItems")
			if items.Exists() {
				parseChannelPlaylistItems(items, &playlists, &nextToken)
			}
			return true
		})
	}

	return &Page[Playlist]{
		Items:     playlists,
		NextToken: nextToken,
		HasMore:   nextToken != "",
	}, nil
}

func parseChannelPlaylistItems(contents gjson.Result, playlists *[]Playlist, nextToken *string) {
	contents.ForEach(func(_, item gjson.Result) bool {
		gpr := item.Get("gridPlaylistRenderer")
		if gpr.Exists() {
			*playlists = append(*playlists, parseGridPlaylistRenderer(gpr))
			return true
		}
		// lockupViewModel for playlists
		lvm := item.Get("lockupViewModel")
		if lvm.Exists() {
			if p, ok := parseLockupPlaylist(lvm); ok {
				*playlists = append(*playlists, p)
			}
			return true
		}
		// Continuation token
		token := item.Get("continuationItemRenderer.continuationEndpoint.continuationCommand.token")
		if token.Exists() {
			*nextToken = token.String()
		}
		return true
	})
}

func parseGridPlaylistRenderer(gpr gjson.Result) Playlist {
	id := gpr.Get("playlistId").String()
	p := Playlist{
		ID:         id,
		Title:      gpr.Get("title.runs.0.text").String(),
		VideoCount: gpr.Get("videoCountShortText.simpleText").String(),
		URL:        PlaylistURL(id),
	}
	// Playlist thumbnails use thumbnailRenderer wrapping, not direct thumbnail
	thumbs := gpr.Get("thumbnailRenderer.playlistVideoThumbnailRenderer.thumbnail.thumbnails")
	if !thumbs.Exists() {
		thumbs = gpr.Get("thumbnailRenderer.playlistCustomThumbnailRenderer.thumbnail.thumbnails")
	}
	if !thumbs.Exists() {
		thumbs = gpr.Get("thumbnail.thumbnails")
	}
	p.Thumbnails = parseThumbnails(thumbs)
	return p
}

func parseLockupPlaylist(lvm gjson.Result) (Playlist, bool) {
	id := lvm.Get("contentId").String()
	if id == "" {
		return Playlist{}, false
	}
	meta := lvm.Get("metadata.lockupMetadataViewModel")
	p := Playlist{
		ID:         id,
		Title:      meta.Get("title.content").String(),
		Thumbnails: parseLockupThumbnails(lvm),
		URL:        PlaylistURL(id),
	}
	// Video count is in the thumbnail overlay badge, not metadata
	lvm.Get("contentImage.collectionThumbnailViewModel.primaryThumbnail.thumbnailViewModel.overlays").ForEach(func(_, overlay gjson.Result) bool {
		overlay.Get("thumbnailOverlayBadgeViewModel.thumbnailBadges").ForEach(func(_, badge gjson.Result) bool {
			text := badge.Get("thumbnailBadgeViewModel.text").String()
			if text != "" {
				p.VideoCount = text
			}
			return true
		})
		return true
	})
	return p, true
}

// parseLockupThumbnails extracts thumbnails from a lockupViewModel's contentImage.
// Tries multiple known paths since YouTube uses different structures for
// videos vs playlists vs collection thumbnails.
func parseLockupThumbnails(lvm gjson.Result) []Thumbnail {
	paths := []string{
		"contentImage.thumbnailViewModel.image.sources",
		"contentImage.thumbnailViewModel.image.thumbnails",
		"contentImage.collectionThumbnailViewModel.primaryThumbnail.thumbnailViewModel.image.sources",
		"contentImage.collectionThumbnailViewModel.primaryThumbnail.thumbnailViewModel.image.thumbnails",
	}
	for _, p := range paths {
		if thumbs := parseThumbnails(lvm.Get(p)); len(thumbs) > 0 {
			return thumbs
		}
	}
	return nil
}

func (c *InnerTubeClient) GetChannelPosts(ctx context.Context, channelID string, pageToken string) (*Page[Post], error) {
	data, err := c.browseChannelTab(ctx, channelID, channelPostsParams, pageToken)
	if err != nil {
		return nil, fmt.Errorf("innertube channel posts: %w", err)
	}

	var posts []Post
	var nextToken string

	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		tr := tab.Get("tabRenderer")
		if !tr.Exists() || !tr.Get("selected").Bool() {
			return true
		}
		sections := tr.Get("content.sectionListRenderer.contents")
		sections.ForEach(func(_, section gjson.Result) bool {
			section.Get("itemSectionRenderer.contents").ForEach(func(_, item gjson.Result) bool {
				bpt := item.Get("backstagePostThreadRenderer.post.backstagePostRenderer")
				if bpt.Exists() {
					posts = append(posts, parseBackstagePost(bpt))
				}
				return true
			})
			// Continuation token
			token := section.Get("continuationItemRenderer.continuationEndpoint.continuationCommand.token")
			if token.Exists() {
				nextToken = token.String()
			}
			return true
		})
		return false
	})

	// Continuation response
	if len(posts) == 0 {
		data.Get("onResponseReceivedActions").ForEach(func(_, action gjson.Result) bool {
			items := action.Get("appendContinuationItemsAction.continuationItems")
			if !items.Exists() {
				return true
			}
			items.ForEach(func(_, item gjson.Result) bool {
				bpt := item.Get("backstagePostThreadRenderer.post.backstagePostRenderer")
				if bpt.Exists() {
					posts = append(posts, parseBackstagePost(bpt))
					return true
				}
				token := item.Get("continuationItemRenderer.continuationEndpoint.continuationCommand.token")
				if token.Exists() {
					nextToken = token.String()
				}
				return true
			})
			return true
		})
	}

	return &Page[Post]{
		Items:     posts,
		NextToken: nextToken,
		HasMore:   nextToken != "",
	}, nil
}

func parseBackstagePost(bpr gjson.Result) Post {
	var b strings.Builder
	bpr.Get("contentText.runs").ForEach(func(_, run gjson.Result) bool {
		b.WriteString(run.Get("text").String())
		return true
	})

	p := Post{
		ID:         bpr.Get("postId").String(),
		AuthorName: bpr.Get("authorText.runs.0.text").String(),
		AuthorID:   bpr.Get("authorEndpoint.browseEndpoint.browseId").String(),
		Content:    b.String(),
		LikeCount:  bpr.Get("voteCount.simpleText").String(),
		PublishedAt: bpr.Get("publishedTimeText.runs.0.text").String(),
	}

	// Comments are accessed via browseEndpoint (FEpost_detail), not continuationCommand
	p.DetailParams = bpr.Get("actionButtons.commentActionButtonsRenderer.replyButton.buttonRenderer.navigationEndpoint.browseEndpoint.params").String()

	// Image from backstageAttachment (single image or first of multi-image)
	att := bpr.Get("backstageAttachment")
	if thumbs := att.Get("backstageImageRenderer.image.thumbnails"); thumbs.Exists() {
		p.Thumbnails = parseThumbnails(thumbs)
	} else if thumbs := att.Get("postMultiImageRenderer.images.0.backstageImageRenderer.image.thumbnails"); thumbs.Exists() {
		p.Thumbnails = parseThumbnails(thumbs)
	}

	return p
}

func (c *InnerTubeClient) GetPlaylistVideos(ctx context.Context, playlistID string, pageToken string) (*Page[Video], error) {
	browseID := "VL" + playlistID
	var cont *string
	if pageToken != "" {
		cont = &pageToken
	}
	raw, err := c.it.Browse(ctx, &browseID, nil, cont)
	if err != nil {
		return nil, fmt.Errorf("innertube playlist videos: %w", err)
	}

	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	var videos []Video
	var nextToken string

	// Initial response: twoColumnBrowseResultsRenderer.tabs[].tabRenderer
	//   .content.sectionListRenderer.contents[].itemSectionRenderer.contents[]
	//   .playlistVideoListRenderer.contents[]
	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		tr := tab.Get("tabRenderer")
		if !tr.Exists() || !tr.Get("selected").Bool() {
			return true
		}
		sections := tr.Get("content.sectionListRenderer.contents")
		sections.ForEach(func(_, section gjson.Result) bool {
			section.Get("itemSectionRenderer.contents").ForEach(func(_, content gjson.Result) bool {
				items := content.Get("playlistVideoListRenderer.contents")
				if items.Exists() {
					parsePlaylistVideoItems(items, &videos, &nextToken)
				}
				return true
			})
			return true
		})
		return false
	})

	// Continuation response
	if len(videos) == 0 {
		data.Get("onResponseReceivedActions").ForEach(func(_, action gjson.Result) bool {
			items := action.Get("appendContinuationItemsAction.continuationItems")
			if items.Exists() {
				parsePlaylistVideoItems(items, &videos, &nextToken)
			}
			return true
		})
	}

	return &Page[Video]{
		Items:     videos,
		NextToken: nextToken,
		HasMore:   nextToken != "",
	}, nil
}

func parsePlaylistVideoItems(contents gjson.Result, videos *[]Video, nextToken *string) {
	contents.ForEach(func(_, item gjson.Result) bool {
		pvr := item.Get("playlistVideoRenderer")
		if pvr.Exists() {
			*videos = append(*videos, parsePlaylistVideoRenderer(pvr))
			return true
		}
		token := item.Get("continuationItemRenderer.continuationEndpoint.continuationCommand.token")
		if token.Exists() {
			*nextToken = token.String()
		}
		return true
	})
}

func parsePlaylistVideoRenderer(pvr gjson.Result) Video {
	v := Video{
		ID:          pvr.Get("videoId").String(),
		Title:       pvr.Get("title.runs.0.text").String(),
		ChannelName: pvr.Get("shortBylineText.runs.0.text").String(),
		ChannelID:   pvr.Get("shortBylineText.runs.0.navigationEndpoint.browseEndpoint.browseId").String(),
		DurationStr: pvr.Get("lengthText.simpleText").String(),
	}
	v.URL = VideoURL(v.ID)
	v.Thumbnails = parseThumbnails(pvr.Get("thumbnail.thumbnails"))
	return v
}

// GetPostComments fetches comments for a community post.
// On initial call, detailParams should be the Post.DetailParams value and
// pageToken should be empty. On continuation, pass the NextToken from the
// previous page as pageToken (detailParams is ignored).
func (c *InnerTubeClient) GetPostComments(ctx context.Context, detailParams string, pageToken string) (*Page[Comment], error) {
	if pageToken != "" {
		// Continuation: browse with the continuation token directly
		raw, err := c.it.Browse(ctx, nil, nil, &pageToken)
		if err != nil {
			return nil, fmt.Errorf("innertube post comments continuation: %w", err)
		}
		return parsePostCommentsResponse(raw)
	}
	if detailParams == "" {
		return &Page[Comment]{}, nil
	}
	// Initial: browse FEpost_detail to get the comment continuation token
	browseID := "FEpost_detail"
	raw, err := c.it.Browse(ctx, &browseID, &detailParams, nil)
	if err != nil {
		return nil, fmt.Errorf("innertube post detail: %w", err)
	}
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}
	// Extract continuation token from the second section
	var commentToken string
	data.Get("contents.twoColumnBrowseResultsRenderer.tabs").ForEach(func(_, tab gjson.Result) bool {
		tr := tab.Get("tabRenderer")
		if !tr.Exists() || !tr.Get("selected").Bool() {
			return true
		}
		tr.Get("content.sectionListRenderer.contents").ForEach(func(_, section gjson.Result) bool {
			section.Get("itemSectionRenderer.contents").ForEach(func(_, item gjson.Result) bool {
				token := item.Get("continuationItemRenderer.continuationEndpoint.continuationCommand.token")
				if token.Exists() {
					commentToken = token.String()
				}
				return true
			})
			return true
		})
		return false
	})
	if commentToken == "" {
		return &Page[Comment]{}, nil
	}
	// Fetch comments using the extracted token
	raw2, err := c.it.Browse(ctx, nil, nil, &commentToken)
	if err != nil {
		return nil, fmt.Errorf("innertube post comments: %w", err)
	}
	return parsePostCommentsResponse(raw2)
}

// Post comments use the same response format as video comments.
var parsePostCommentsResponse = parseCommentsResponse

func (c *InnerTubeClient) IsAuthenticated() bool {
	return c.authenticated
}

// parseSearchResponse extracts videos from an InnerTube search response.
// Handles both initial responses (contents path) and continuation responses
// (onResponseReceivedCommands path).
func parseSearchResponse(raw map[string]interface{}) (*Page[Video], error) {
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	var videos []Video
	var nextToken string

	// Initial response: contents.twoColumnSearchResultsRenderer...
	sections := data.Get("contents.twoColumnSearchResultsRenderer.primaryContents.sectionListRenderer.contents")

	// Continuation response: onResponseReceivedCommands[].appendContinuationItemsAction.continuationItems
	if !sections.Exists() {
		data.Get("onResponseReceivedCommands").ForEach(func(_, cmd gjson.Result) bool {
			items := cmd.Get("appendContinuationItemsAction.continuationItems")
			if items.Exists() {
				sections = items
			}
			return true
		})
	}

	sections.ForEach(func(_, section gjson.Result) bool {
		section.Get("itemSectionRenderer.contents").ForEach(func(_, item gjson.Result) bool {
			vr := item.Get("videoRenderer")
			if !vr.Exists() {
				return true
			}
			videos = append(videos, parseVideoRenderer(vr))
			return true
		})

		token := section.Get("continuationItemRenderer.continuationEndpoint.continuationCommand.token")
		if token.Exists() {
			nextToken = token.String()
		}

		return true
	})

	return &Page[Video]{
		Items:     videos,
		NextToken: nextToken,
		HasMore:   nextToken != "",
	}, nil
}

// parseVideoRenderer extracts a Video from a videoRenderer JSON object.
func parseVideoRenderer(vr gjson.Result) Video {
	v := Video{
		ID:          vr.Get("videoId").String(),
		Title:       vr.Get("title.runs.0.text").String(),
		ChannelName: vr.Get("ownerText.runs.0.text").String(),
		ChannelID:   vr.Get("ownerText.runs.0.navigationEndpoint.browseEndpoint.browseId").String(),
		DurationStr: vr.Get("lengthText.simpleText").String(),
		ViewCount:   vr.Get("viewCountText.simpleText").String(),
		PublishedAt: vr.Get("publishedTimeText.simpleText").String(),
	}
	v.URL = VideoURL(v.ID)
	v.Thumbnails = parseThumbnails(vr.Get("thumbnail.thumbnails"))
	return v
}

// parseLockupViewModel extracts a Video from the new lockupViewModel format
// used in subscription feeds.
func parseLockupViewModel(lvm gjson.Result) Video {
	meta := lvm.Get("metadata.lockupMetadataViewModel")

	v := Video{
		ID:    lvm.Get("contentId").String(),
		Title: meta.Get("title.content").String(),
	}
	v.URL = VideoURL(v.ID)

	// Channel name from first metadata row
	v.ChannelName = meta.Get("metadata.contentMetadataViewModel.metadataRows.0.metadataParts.0.text.content").String()

	// Channel ID from command runs in metadata
	meta.Get("metadata.contentMetadataViewModel.metadataRows").ForEach(func(_, row gjson.Result) bool {
		row.Get("metadataParts").ForEach(func(_, part gjson.Result) bool {
			part.Get("text.commandRuns").ForEach(func(_, cmd gjson.Result) bool {
				browseID := cmd.Get("onTap.innertubeCommand.browseEndpoint.browseId").String()
				if browseID != "" {
					v.ChannelID = browseID
				}
				return true
			})
			return true
		})
		return true
	})

	// Views and published from metadata rows (row index 1+)
	meta.Get("metadata.contentMetadataViewModel.metadataRows").ForEach(func(i gjson.Result, row gjson.Result) bool {
		if i.Int() == 0 {
			return true // skip channel name row
		}
		row.Get("metadataParts").ForEach(func(_, part gjson.Result) bool {
			txt := part.Get("text.content").String()
			if txt == "" {
				return true
			}
			if v.ViewCount == "" {
				v.ViewCount = txt
			} else if v.PublishedAt == "" {
				v.PublishedAt = txt
			}
			return true
		})
		return true
	})

	v.DurationStr = lvm.Get("contentImage.thumbnailViewModel.overlays.0.thumbnailBottomOverlayViewModel.badges.0.thumbnailBadgeViewModel.text").String()
	v.Thumbnails = parseLockupThumbnails(lvm)

	return v
}

func toGJSON(raw map[string]interface{}) (gjson.Result, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return gjson.Result{}, fmt.Errorf("marshal innertube response: %w", err)
	}
	return gjson.ParseBytes(data), nil
}

func formatSeconds(s string) string {
	total, err := strconv.Atoi(s)
	if err != nil || total < 0 {
		return s
	}
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}
