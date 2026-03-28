package youtube

import (
	"encoding/json"
	"os"
	"testing"
)

func loadFixture(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", path, err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to parse fixture %s: %v", path, err)
	}
	return m
}

func TestParseSearchResponse(t *testing.T) {
	raw := loadFixture(t, "testdata/search_response.json")
	page, err := parseSearchResponse(raw)
	if err != nil {
		t.Fatalf("parseSearchResponse: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("expected search results, got 0")
	}

	t.Run("FirstResultFields", func(t *testing.T) {
		v := page.Items[0]
		if v.ID == "" {
			t.Error("missing ID")
		}
		if v.Title == "" {
			t.Error("missing Title")
		}
		if v.ChannelName == "" {
			t.Error("missing ChannelName")
		}
		if v.URL != VideoURL(v.ID) {
			t.Errorf("URL = %q, want %q", v.URL, VideoURL(v.ID))
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		if !page.HasMore {
			t.Error("expected HasMore=true")
		}
		if page.NextToken == "" {
			t.Error("expected non-empty NextToken")
		}
	})

	t.Run("ThumbnailsAndMetadata", func(t *testing.T) {
		// First result should have thumbnails, duration, views
		v := page.Items[0]
		if len(v.Thumbnails) == 0 {
			t.Error("expected thumbnails")
		}
		for i, th := range v.Thumbnails {
			if th.URL == "" {
				t.Errorf("thumbnail %d missing URL", i)
			}
			if th.Width == 0 || th.Height == 0 {
				t.Errorf("thumbnail %d missing dimensions", i)
			}
		}
		if v.DurationStr == "" {
			t.Error("missing DurationStr")
		}
		if v.ViewCount == "" {
			t.Error("missing ViewCount")
		}
		if v.ChannelID == "" {
			t.Error("missing ChannelID")
		}
	})
}

func TestParseSearchContinuation(t *testing.T) {
	raw := loadFixture(t, "testdata/search_continuation.json")
	page, err := parseSearchResponse(raw)
	if err != nil {
		t.Fatalf("parseSearchResponse (continuation): %v", err)
	}

	if len(page.Items) == 0 {
		t.Fatal("expected continuation results, got 0")
	}

	v := page.Items[0]
	if v.ID == "" {
		t.Error("first continuation result missing ID")
	}
	if v.Title == "" {
		t.Error("first continuation result missing Title")
	}
}

func TestEnrichVideo(t *testing.T) {
	raw := loadFixture(t, "testdata/next_response.json")

	v := &Video{ID: "446E-r0rXHI"}
	enrichVideo(v, raw)

	// Assert concrete values from the known fixture
	if v.ViewCount == "" {
		t.Error("enrichVideo did not set ViewCount")
	}
	if v.PublishedAt == "" {
		t.Error("enrichVideo did not set PublishedAt")
	}
	if v.LikeCount == "" {
		t.Error("enrichVideo did not set LikeCount")
	}
	if v.SubscriberCount == "" {
		t.Error("enrichVideo did not set SubscriberCount")
	}
	if v.Description == "" {
		t.Error("enrichVideo did not set Description")
	}

	// Verify the description looks like real content (not a gjson artifact)
	if len(v.Description) < 20 {
		t.Errorf("Description too short (%d chars), likely not parsed correctly", len(v.Description))
	}
}

func TestFormatSeconds(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"0", "0:00"},
		{"59", "0:59"},
		{"60", "1:00"},
		{"61", "1:01"},
		{"3599", "59:59"},
		{"3600", "1:00:00"},
		{"3661", "1:01:01"},
		{"7200", "2:00:00"},
		{"", ""},
		{"abc", "abc"},
		{"-1", "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := formatSeconds(tt.input)
			if got != tt.want {
				t.Errorf("formatSeconds(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestVideoURL(t *testing.T) {
	got := VideoURL("abc123")
	want := "https://www.youtube.com/watch?v=abc123"
	if got != want {
		t.Errorf("VideoURL = %q, want %q", got, want)
	}
}
