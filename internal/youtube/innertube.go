package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	innertubego "github.com/nezbut/innertube-go"
	"github.com/tidwall/gjson"
)

const searchFilterVideosOnly = "EgIQAQ%3D%3D"

// VideoURL returns the canonical watch URL for a video ID.
func VideoURL(id string) string {
	return "https://www.youtube.com/watch?v=" + id
}

// InnerTubeClient implements Client using YouTube's InnerTube API.
type InnerTubeClient struct {
	it            *innertubego.InnerTube
	authenticated bool
}

// NewInnerTubeClient creates a new InnerTube-backed client.
// Pass a custom httpClient with a cookie jar for authenticated requests.
func NewInnerTubeClient(httpClient *http.Client) (*InnerTubeClient, error) {
	it, err := innertubego.NewInnerTube(httpClient, "WEB", "2.20240101.00.00", "", "", "", nil, true)
	if err != nil {
		return nil, fmt.Errorf("innertube init: %w", err)
	}
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
	return nil, fmt.Errorf("not implemented")
}

func (c *InnerTubeClient) GetReplies(ctx context.Context, commentID string, pageToken string) (*Page[Comment], error) {
	return nil, fmt.Errorf("not implemented")
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

func (c *InnerTubeClient) GetFeed(ctx context.Context) (*Page[Video], error) {
	if !c.authenticated {
		return nil, fmt.Errorf("authentication required for subscription feed")
	}

	browseID := "FEsubscriptions"
	raw, err := c.it.Browse(ctx, &browseID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("innertube browse subscriptions: %w", err)
	}

	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	var videos []Video
	var nextToken string

	// Navigate to the selected tab's richGridRenderer
	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		tr := tab.Get("tabRenderer")
		if !tr.Exists() || !tr.Get("selected").Bool() {
			return true
		}
		contents := tr.Get("content.richGridRenderer.contents")
		contents.ForEach(func(_, item gjson.Result) bool {
			// Classic videoRenderer
			vr := item.Get("richItemRenderer.content.videoRenderer")
			if vr.Exists() {
				videos = append(videos, parseVideoRenderer(vr))
				return true
			}
			// New lockupViewModel format
			lvm := item.Get("richItemRenderer.content.lockupViewModel")
			if lvm.Exists() {
				videos = append(videos, parseLockupViewModel(lvm))
				return true
			}
			// Continuation token
			token := item.Get("continuationItemRenderer.continuationEndpoint.continuationCommand.token")
			if token.Exists() {
				nextToken = token.String()
			}
			return true
		})
		return false // stop after selected tab
	})

	return &Page[Video]{
		Items:     videos,
		NextToken: nextToken,
		HasMore:   nextToken != "",
	}, nil
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
