package youtube

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestParseLockupViewModel(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_feed_response.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatal(err)
	}

	var videos []Video
	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		contents := tab.Get("tabRenderer.content.richGridRenderer.contents")
		contents.ForEach(func(_, item gjson.Result) bool {
			lvm := item.Get("richItemRenderer.content.lockupViewModel")
			if lvm.Exists() {
				videos = append(videos, parseLockupViewModel(lvm))
			}
			return true
		})
		return true
	})

	if len(videos) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(videos))
	}

	t.Run("FirstVideo", func(t *testing.T) {
		v := videos[0]
		assertEqual(t, "ID", v.ID, "fake_vid_001")
		assertEqual(t, "Title", v.Title, "Fake Video Title One")
		assertEqual(t, "ChannelName", v.ChannelName, "Fake Channel")
		assertEqual(t, "ChannelID", v.ChannelID, "UCfake123")
		assertEqual(t, "ViewCount", v.ViewCount, "10K views")
		assertEqual(t, "PublishedAt", v.PublishedAt, "2 hours ago")
		assertEqual(t, "DurationStr", v.DurationStr, "12:34")
		assertEqual(t, "URL", v.URL, VideoURL("fake_vid_001"))
	})

	t.Run("SecondVideo", func(t *testing.T) {
		v := videos[1]
		assertEqual(t, "ID", v.ID, "fake_vid_002")
		assertEqual(t, "Title", v.Title, "Another Fake Video")
		assertEqual(t, "ChannelName", v.ChannelName, "Another Channel")
		assertEqual(t, "ViewCount", v.ViewCount, "500 views")
		assertEqual(t, "PublishedAt", v.PublishedAt, "1 day ago")
		assertEqual(t, "DurationStr", v.DurationStr, "5:00")
	})
}

func TestParseLockupViewModel_FeedContinuation(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_feed_response.json")
	data, _ := toGJSON(raw)

	var nextToken string
	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		contents := tab.Get("tabRenderer.content.richGridRenderer.contents")
		contents.ForEach(func(_, item gjson.Result) bool {
			ci := item.Get("continuationItemRenderer")
			if ci.Exists() {
				nextToken = extractContinuationToken(ci)
			}
			return true
		})
		return true
	})

	if nextToken != "fake_continuation_token_feed" {
		t.Errorf("expected feed continuation token, got %q", nextToken)
	}
}

func TestParseChannelSections(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_channels_response.json")
	data, _ := toGJSON(raw)

	var channels []Channel
	var nextToken string
	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		sections := tab.Get("tabRenderer.content.sectionListRenderer.contents")
		parseChannelSections(sections, &channels, &nextToken)
		return true
	})

	if len(channels) != 3 {
		t.Fatalf("expected 3 channels, got %d", len(channels))
	}

	t.Run("NonTopicChannel", func(t *testing.T) {
		ch := channels[0]
		assertEqual(t, "ID", ch.ID, "UCfake_ch1")
		assertEqual(t, "Name", ch.Name, "Fake Channel One")
		assertEqual(t, "Handle", ch.Handle, "@fakechannel1")
		assertEqual(t, "SubscriberCount", ch.SubscriberCount, "100K subscribers")
		if len(ch.Thumbnails) != 1 {
			t.Errorf("expected 1 thumbnail, got %d", len(ch.Thumbnails))
		}
	})

	t.Run("TopicChannel", func(t *testing.T) {
		ch := channels[1]
		assertEqual(t, "ID", ch.ID, "UCfake_ch2")
		assertEqual(t, "Name", ch.Name, "Topic Channel - Topic")
		assertEqual(t, "Handle", ch.Handle, "")
		assertEqual(t, "SubscriberCount", ch.SubscriberCount, "50K subscribers")
	})

	t.Run("NoVideoCountText", func(t *testing.T) {
		ch := channels[2]
		assertEqual(t, "Handle", ch.Handle, "@thirdchannel")
		assertEqual(t, "SubscriberCount", ch.SubscriberCount, "1.2M subscribers")
	})

	t.Run("ContinuationToken", func(t *testing.T) {
		if nextToken != "fake_channels_continuation" {
			t.Errorf("expected continuation token, got %q", nextToken)
		}
	})
}

func TestParseCommentsResponse(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_comments_response.json")
	page, err := parseCommentsResponse(raw)
	if err != nil {
		t.Fatal(err)
	}

	if len(page.Items) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(page.Items))
	}

	t.Run("FirstComment", func(t *testing.T) {
		c := page.Items[0]
		assertEqual(t, "ID", c.ID, "fake_comment_id_1")
		assertEqual(t, "AuthorName", c.AuthorName, "FakeUser1")
		assertEqual(t, "AuthorID", c.AuthorID, "UCfake_user1")
		assertEqual(t, "Content", c.Content, "This is a fake comment with some text")
		assertEqual(t, "LikeCount", c.LikeCount, "42")
		assertEqual(t, "PublishedAt", c.PublishedAt, "2 hours ago")
		assertEqual(t, "ReplyToken", c.ReplyToken, "fake_reply_token_1")
		if c.ReplyCount != 5 {
			t.Errorf("ReplyCount = %d, want 5", c.ReplyCount)
		}
		if c.IsOwner {
			t.Error("expected IsOwner=false")
		}
	})

	t.Run("OwnerComment", func(t *testing.T) {
		c := page.Items[1]
		assertEqual(t, "ID", c.ID, "fake_comment_id_2")
		assertEqual(t, "AuthorName", c.AuthorName, "ChannelOwner")
		if !c.IsOwner {
			t.Error("expected IsOwner=true")
		}
		if c.ReplyToken != "" {
			t.Errorf("expected empty ReplyToken, got %q", c.ReplyToken)
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		if !page.HasMore {
			t.Error("expected HasMore=true")
		}
		assertEqual(t, "NextToken", page.NextToken, "fake_comments_next_page")
	})
}

func TestParseRepliesResponse(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_replies_response.json")
	page, err := parseCommentsResponse(raw)
	if err != nil {
		t.Fatal(err)
	}

	if len(page.Items) != 2 {
		t.Fatalf("expected 2 replies, got %d", len(page.Items))
	}

	t.Run("FirstReply", func(t *testing.T) {
		r := page.Items[0]
		assertEqual(t, "ID", r.ID, "fake_reply_id_1")
		assertEqual(t, "AuthorName", r.AuthorName, "ReplyUser1")
		assertEqual(t, "Content", r.Content, "This is a reply")
		assertEqual(t, "LikeCount", r.LikeCount, "5")
	})

	t.Run("ReplyWithEmptyLikes", func(t *testing.T) {
		r := page.Items[1]
		assertEqual(t, "LikeCount", r.LikeCount, "")
		if !r.IsOwner {
			t.Error("expected IsOwner=true for creator reply")
		}
	})

	t.Run("ButtonContinuationToken", func(t *testing.T) {
		if !page.HasMore {
			t.Error("expected HasMore=true")
		}
		assertEqual(t, "NextToken", page.NextToken, "fake_more_replies_token")
	})
}

func TestExtractCommentsToken(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_next_response.json")
	token := extractCommentsToken(raw)
	if token != "fake_comments_token_abc123" {
		t.Errorf("extractCommentsToken = %q, want %q", token, "fake_comments_token_abc123")
	}
}

func TestEnrichVideoFromFakeFixture(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_next_response.json")
	v := &Video{ID: "fake_vid"}
	enrichVideo(v, raw)

	assertEqual(t, "ViewCount", v.ViewCount, "1,234,567 views")
	assertEqual(t, "PublishedAt", v.PublishedAt, "Jan 1, 2025")
	assertEqual(t, "LikeCount", v.LikeCount, "10K")
	assertEqual(t, "SubscriberCount", v.SubscriberCount, "500K subscribers")
	assertEqual(t, "Description", v.Description, "This is a fake video description for testing purposes.")
}

func TestExtractContinuationToken(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "continuationEndpoint path",
			json: `{"continuationEndpoint":{"continuationCommand":{"token":"token_a"}}}`,
			want: "token_a",
		},
		{
			name: "button path",
			json: `{"button":{"buttonRenderer":{"command":{"continuationCommand":{"token":"token_b"}}}}}`,
			want: "token_b",
		},
		{
			name: "empty",
			json: `{}`,
			want: "",
		},
		{
			name: "both paths prefers continuationEndpoint",
			json: `{"continuationEndpoint":{"continuationCommand":{"token":"primary"}},"button":{"buttonRenderer":{"command":{"continuationCommand":{"token":"fallback"}}}}}`,
			want: "primary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gjson.Parse(tt.json)
			got := extractContinuationToken(r)
			if got != tt.want {
				t.Errorf("extractContinuationToken = %q, want %q", got, tt.want)
			}
		})
	}
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", field, got, want)
	}
}
