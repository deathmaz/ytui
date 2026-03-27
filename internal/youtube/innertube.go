package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

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
	it *innertubego.InnerTube
}

// NewInnerTubeClient creates a new InnerTube-backed client.
// Pass a custom httpClient with a cookie jar for authenticated requests.
func NewInnerTubeClient(httpClient *http.Client) (*InnerTubeClient, error) {
	it, err := innertubego.NewInnerTube(httpClient, "WEB", "2.20240101.00.00", "", "", "", nil, true)
	if err != nil {
		return nil, fmt.Errorf("innertube init: %w", err)
	}
	return &InnerTubeClient{it: it}, nil
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
	raw, err := c.it.Player(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("innertube player: %w", err)
	}

	data, err := toGJSON(raw)
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

	return v, nil
}

func (c *InnerTubeClient) GetComments(ctx context.Context, videoID string, pageToken string) (*Page[Comment], error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *InnerTubeClient) GetReplies(ctx context.Context, commentID string, pageToken string) (*Page[Comment], error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *InnerTubeClient) GetSubscriptions(ctx context.Context, pageToken string) (*Page[Channel], error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *InnerTubeClient) GetFeed(ctx context.Context) (*Page[Video], error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *InnerTubeClient) GetChannelVideos(ctx context.Context, channelID string, pageToken string) (*Page[Video], error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *InnerTubeClient) IsAuthenticated() bool {
	return false
}

// parseSearchResponse extracts videos from an InnerTube search response.
func parseSearchResponse(raw map[string]interface{}) (*Page[Video], error) {
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	var videos []Video
	var nextToken string

	sections := data.Get("contents.twoColumnSearchResultsRenderer.primaryContents.sectionListRenderer.contents")
	sections.ForEach(func(_, section gjson.Result) bool {
		// Video items
		section.Get("itemSectionRenderer.contents").ForEach(func(_, item gjson.Result) bool {
			vr := item.Get("videoRenderer")
			if !vr.Exists() {
				return true
			}
			videos = append(videos, parseVideoRenderer(vr))
			return true
		})

		// Continuation token
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

func toGJSON(raw map[string]interface{}) (gjson.Result, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return gjson.Result{}, fmt.Errorf("marshal innertube response: %w", err)
	}
	return gjson.ParseBytes(data), nil
}

func formatSeconds(s string) string {
	var total int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			total = total*10 + int(c-'0')
		}
	}
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}
