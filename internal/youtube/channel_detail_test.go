package youtube

import (
	"testing"
)

func TestParseChannelDetail_Subscribed(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_channel_detail_subscribed.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	d := parseChannelDetail(data)

	assertEqual(t, "ID", d.ID, "UCfake123456789abcdef_AB")
	assertEqual(t, "Name", d.Name, "Fake Channel")
	assertEqual(t, "Handle", d.Handle, "@fakechannel")
	assertEqual(t, "Description", d.Description, "Synthetic fixture channel for tests.")
	assertEqual(t, "SubscriberCount", d.SubscriberCount, "1.2M subscribers")
	assertEqual(t, "VideoCount", d.VideoCount, "123 videos")
	assertEqual(t, "URL", d.URL, "https://www.youtube.com/channel/UCfake123456789abcdef_AB")
	if !d.SubscribedKnown {
		t.Fatal("SubscribedKnown = false, want true")
	}
	if !d.Subscribed {
		t.Fatal("Subscribed = false, want true")
	}
	if len(d.Thumbnails) != 1 || d.Thumbnails[0].URL != "https://fake.test/avatar.jpg" {
		t.Fatalf("Thumbnails = %+v", d.Thumbnails)
	}
}

func TestParseChannelDetail_NotSubscribed(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_channel_detail_not_subscribed.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	d := parseChannelDetail(data)

	assertEqual(t, "ID", d.ID, "UCfake987654321zyxwvu_CD")
	assertEqual(t, "Handle", d.Handle, "@anotherfake")
	assertEqual(t, "SubscriberCount", d.SubscriberCount, "4.5K subscribers")
	assertEqual(t, "VideoCount", d.VideoCount, "17 videos")
	if !d.SubscribedKnown {
		t.Fatal("SubscribedKnown = false, want true")
	}
	if d.Subscribed {
		t.Fatal("Subscribed = true, want false")
	}
}

// When the channel metadata lacks a description, the parser should fall back
// to the header's description preview.
func TestParseChannelDetail_DescriptionFallback(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_channel_detail_not_subscribed.json")
	data, _ := toGJSON(raw)
	d := parseChannelDetail(data)
	assertEqual(t, "Description (fallback)", d.Description, "Fallback description only in header.")
}

// Unauthenticated responses omit the state entity. SubscribedKnown must be
// false so callers can hide the indicator rather than misreport "not
// subscribed."
func TestParseChannelDetail_Unauthenticated(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_channel_detail_unauthenticated.json")
	data, _ := toGJSON(raw)
	d := parseChannelDetail(data)

	assertEqual(t, "ID", d.ID, "UCfake000000000noauth_EF")
	if d.SubscribedKnown {
		t.Fatal("SubscribedKnown = true, want false (no state entity)")
	}
	if d.Subscribed {
		t.Fatal("Subscribed = true, want false")
	}
}
