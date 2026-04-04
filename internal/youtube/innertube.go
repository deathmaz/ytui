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

// WEB client config — bypasses innertube-go's stale hardcoded defaults
// which YouTube now rejects with HTML responses.
const (
	webClientName    = "WEB"
	webClientVersion = "2.20260401.00.00"
	webClientID      = 1
	webAPIKey        = "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"
	webReferer       = "https://www.youtube.com/"
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
			ClientVersion: webClientVersion,
			ClientID:      webClientID,
			APIKey:        webAPIKey,
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

	vd.Get("thumbnail.thumbnails").ForEach(func(_, t gjson.Result) bool {
		v.Thumbnails = append(v.Thumbnails, Thumbnail{
			URL:    t.Get("url").String(),
			Width:  int(t.Get("width").Int()),
			Height: int(t.Get("height").Int()),
		})
		return true
	})

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
				cr.Get("thumbnail.thumbnails").ForEach(func(_, t gjson.Result) bool {
					ch.Thumbnails = append(ch.Thumbnails, Thumbnail{
						URL:    t.Get("url").String(),
						Width:  int(t.Get("width").Int()),
						Height: int(t.Get("height").Int()),
					})
					return true
				})
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

func parseFeedItems(contents gjson.Result, videos *[]Video, nextToken *string) {
	contents.ForEach(func(_, item gjson.Result) bool {
		// Classic videoRenderer
		vr := item.Get("richItemRenderer.content.videoRenderer")
		if vr.Exists() {
			*videos = append(*videos, parseVideoRenderer(vr))
			return true
		}
		// New lockupViewModel format
		lvm := item.Get("richItemRenderer.content.lockupViewModel")
		if lvm.Exists() {
			*videos = append(*videos, parseLockupViewModel(lvm))
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

func (c *InnerTubeClient) GetChannelVideos(ctx context.Context, channelID string, pageToken string) (*Page[Video], error) {
	return nil, fmt.Errorf("not implemented")
}

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
	channelID := vr.Get("ownerText.runs.0.navigationEndpoint.browseEndpoint.browseId").String()

	v := Video{
		ID:          vr.Get("videoId").String(),
		Title:       vr.Get("title.runs.0.text").String(),
		ChannelName: vr.Get("ownerText.runs.0.text").String(),
		ChannelID:   channelID,
		DurationStr: vr.Get("lengthText.simpleText").String(),
		ViewCount:   vr.Get("viewCountText.simpleText").String(),
		PublishedAt: vr.Get("publishedTimeText.simpleText").String(),
	}
	v.URL = VideoURL(v.ID)

	vr.Get("thumbnail.thumbnails").ForEach(func(_, t gjson.Result) bool {
		v.Thumbnails = append(v.Thumbnails, Thumbnail{
			URL:    t.Get("url").String(),
			Width:  int(t.Get("width").Int()),
			Height: int(t.Get("height").Int()),
		})
		return true
	})

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

	// Duration from thumbnail overlay badge
	v.DurationStr = lvm.Get("contentImage.thumbnailViewModel.overlays.0.thumbnailBottomOverlayViewModel.badges.0.thumbnailBadgeViewModel.text").String()

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
