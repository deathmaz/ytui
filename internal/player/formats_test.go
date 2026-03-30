package player

import (
	"testing"
)

func TestParseResolutions(t *testing.T) {
	output := `[youtube] Extracting URL: https://www.youtube.com/watch?v=fake123
[youtube] fake123: Downloading webpage
ID      EXT   RESOLUTION FPS CH │   FILESIZE   TBR PROTO │ VCODEC          VBR ACODEC      ABR ASR MORE INFO
──────────────────────────────────────────────────────────────────────────────────────────────────────────────
sb2     mhtml 48x27        0    │                  mhtml │ images                                  storyboard
sb1     mhtml 80x45        0    │                  mhtml │ images                                  storyboard
233     mp4   audio only        │                  m3u8  │ audio only          mp4a.40.2       HLS
234     mp4   audio only        │                  m3u8  │ audio only          mp4a.40.2       HLS
599     m4a   audio only      2 │ ~ 16.42MiB   31k https │ audio only          mp4a.40.5   31k 22k low, m4a_dash
600     webm  audio only      2 │ ~ 17.03MiB   32k https │ audio only          opus        32k 48k low, webm_dash
139     m4a   audio only      2 │   27.60MiB   52k https │ audio only          mp4a.40.5   52k 22k m4a_dash
249     webm  audio only      2 │   27.98MiB   53k https │ audio only          opus        53k 48k webm_dash
250     webm  audio only      2 │   37.47MiB   71k https │ audio only          opus        71k 48k webm_dash
140     m4a   audio only      2 │   73.24MiB  139k https │ audio only          mp4a.40.2  139k 44k medium, m4a_dash
251     webm  audio only      2 │   73.16MiB  138k https │ audio only          opus       138k 48k medium, webm_dash
269     mp4   256x144     30    │ ~  6.89MiB   13k m3u8  │ avc1.4D400B     13k video only          144p, HLS
160     mp4   256x144     30    │   17.69MiB   33k https │ avc1.4D400B     33k video only          144p, mp4_dash
603     mp4   256x144     30    │ ~ 10.07MiB   19k m3u8  │ vp09.00.11.08   19k video only          144p, HLS
278     webm  256x144     30    │   20.49MiB   38k https │ vp09.00.11.08   38k video only          144p, webm_dash
229     mp4   426x240     30    │ ~ 12.46MiB   23k m3u8  │ avc1.4D4015     23k video only          240p, HLS
133     mp4   426x240     30    │   33.17MiB   63k https │ avc1.4D4015     63k video only          240p, mp4_dash
604     mp4   426x240     30    │ ~ 17.06MiB   32k m3u8  │ vp09.00.20.08   32k video only          240p, HLS
242     webm  426x240     30    │   39.50MiB   75k https │ vp09.00.20.08   75k video only          240p, webm_dash
230     mp4   640x360     30    │ ~ 24.53MiB   46k m3u8  │ avc1.4D401E     46k video only          360p, HLS
134     mp4   640x360     30    │   60.44MiB  114k https │ avc1.4D401E    114k video only          360p, mp4_dash
18      mp4   640x360     30  2 │  125.03MiB  237k https │ avc1.42001E    237k mp4a.40.2    0k 44k 360p
605     mp4   640x360     30    │ ~ 27.50MiB   52k m3u8  │ vp09.00.21.08   52k video only          360p, HLS
243     webm  640x360     30    │   61.38MiB  116k https │ vp09.00.21.08  116k video only          360p, webm_dash
231     mp4   854x480     30    │ ~ 39.36MiB   74k m3u8  │ avc1.4D401F     74k video only          480p, HLS
135     mp4   854x480     30    │   95.00MiB  180k https │ avc1.4D401F    180k video only          480p, mp4_dash
606     mp4   854x480     30    │ ~ 39.63MiB   75k m3u8  │ vp09.00.30.08   75k video only          480p, HLS
244     webm  854x480     30    │   81.87MiB  155k https │ vp09.00.30.08  155k video only          480p, webm_dash
232     mp4   1280x720    30    │ ~ 73.13MiB  138k m3u8  │ avc1.4D401F    138k video only          720p, HLS
136     mp4   1280x720    30    │  179.26MiB  339k https │ avc1.4D401F    339k video only          720p, mp4_dash
609     mp4   1280x720    30    │ ~ 66.64MiB  126k m3u8  │ vp09.00.31.08  126k video only          720p, HLS
247     webm  1280x720    30    │  149.63MiB  283k https │ vp09.00.31.08  283k video only          720p, webm_dash
270     mp4   1920x1080   30    │ ~128.34MiB  243k m3u8  │ avc1.640028    243k video only          1080p, HLS
137     mp4   1920x1080   30    │  339.42MiB  643k https │ avc1.640028    643k video only          1080p, mp4_dash
612     mp4   1920x1080   30    │ ~105.49MiB  199k m3u8  │ vp09.00.40.08  199k video only          1080p, HLS
248     webm  1920x1080   30    │  219.81MiB  416k https │ vp09.00.40.08  416k video only          1080p, webm_dash
22      mp4   1280x720    30  2 │ ≈ 252.49MiB  478k https │ avc1.64001F    478k mp4a.40.2    0k 44k 720p
`

	heights := parseResolutions(output)

	if len(heights) == 0 {
		t.Fatal("expected resolutions, got none")
	}

	// Should be sorted descending
	for i := 1; i < len(heights); i++ {
		if heights[i] >= heights[i-1] {
			t.Errorf("not sorted descending: %d >= %d at index %d", heights[i], heights[i-1], i)
		}
	}

	// Should contain expected resolutions
	expected := map[int]bool{1080: true, 720: true, 480: true, 360: true, 240: true, 144: true}
	for _, h := range heights {
		delete(expected, h)
	}
	if len(expected) > 0 {
		t.Errorf("missing resolutions: %v", expected)
	}

	// Should be deduplicated
	seen := map[int]bool{}
	for _, h := range heights {
		if seen[h] {
			t.Errorf("duplicate resolution: %d", h)
		}
		seen[h] = true
	}
}

func TestParseResolutions_Empty(t *testing.T) {
	heights := parseResolutions("")
	if len(heights) != 0 {
		t.Errorf("expected empty, got %v", heights)
	}
}

func TestParseResolutions_AudioOnly(t *testing.T) {
	output := `ID  EXT RESOLUTION
───────────────────
251 webm audio only
140 m4a  audio only`

	heights := parseResolutions(output)
	if len(heights) != 0 {
		t.Errorf("audio-only should produce no resolutions, got %v", heights)
	}
}

func TestDefaultFormats(t *testing.T) {
	formats := DefaultFormats()
	if len(formats) == 0 {
		t.Fatal("expected default formats")
	}
	if formats[0].ID != "best" {
		t.Errorf("first format should be 'best', got %q", formats[0].ID)
	}
	// Should have audio option
	hasAudio := false
	for _, f := range formats {
		if f.ID == "audio" {
			hasAudio = true
		}
	}
	if !hasAudio {
		t.Error("missing audio-only option")
	}
}
