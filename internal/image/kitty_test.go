package image

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestEncodeForKitty(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}

	tx, pl, err := EncodeForKitty(img, 5, 3)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("TransmitSequence", func(t *testing.T) {
		// Must start with ESC_G and contain transmit params
		if !strings.Contains(tx, "\x1b_G") {
			t.Error("transmit missing ESC_G")
		}
		if !strings.Contains(tx, "a=t") {
			t.Error("transmit should use a=t (transmit only, not display)")
		}
		if !strings.Contains(tx, "U=1") {
			t.Error("transmit should not use U=1 (that's for placement)")
		}
		if !strings.Contains(tx, "f=100") {
			t.Error("transmit missing f=100 (PNG format)")
		}
		if !strings.Contains(tx, "q=2") {
			t.Error("transmit missing q=2 (quiet mode)")
		}
		// Must contain virtual placement command
		if !strings.Contains(tx, "a=p") {
			t.Error("transmit missing virtual placement (a=p)")
		}
		if !strings.Contains(tx, "c=5") {
			t.Error("placement missing c=5")
		}
		if !strings.Contains(tx, "r=3") {
			t.Error("placement missing r=3")
		}
		// Must end with string terminator
		if !strings.HasSuffix(tx, "\x1b\\") {
			t.Error("transmit must end with ESC\\")
		}
	})

	t.Run("PlaceholderGrid", func(t *testing.T) {
		lines := strings.Split(pl, "\n")
		if len(lines) != 3 {
			t.Fatalf("expected 3 rows, got %d", len(lines))
		}
		for i, line := range lines {
			count := strings.Count(line, "\U0010EEEE")
			if count != 5 {
				t.Errorf("row %d: expected 5 placeholders, got %d", i, count)
			}
		}
	})

	t.Run("PlaceholderHasForegroundColor", func(t *testing.T) {
		// Must contain 24-bit foreground color escape for image ID
		if !strings.Contains(pl, "\033[38;2;") {
			t.Error("placeholder missing 24-bit foreground color escape")
		}
		// Must contain color reset
		if !strings.Contains(pl, "\033[39m") {
			t.Error("placeholder missing color reset")
		}
	})

	t.Run("FirstCellHasRowDiacritic", func(t *testing.T) {
		lines := strings.Split(pl, "\n")
		// First cell of each row should have a diacritic after U+10EEEE
		for i, line := range lines {
			// Find first U+10EEEE in the line
			idx := strings.Index(line, "\U0010EEEE")
			if idx < 0 {
				t.Fatalf("row %d: no placeholder found", i)
			}
			// The character after U+10EEEE should be a diacritic
			afterPlaceholder := line[idx+len("\U0010EEEE"):]
			if len(afterPlaceholder) == 0 {
				t.Fatalf("row %d: nothing after first placeholder", i)
			}
			firstRune := []rune(afterPlaceholder)[0]
			if i < len(diacritics) && firstRune != diacritics[i] {
				t.Errorf("row %d: expected diacritic U+%04X, got U+%04X", i, diacritics[i], firstRune)
			}
		}
	})
}

func TestEncodeForKitty_LargeImage(t *testing.T) {
	// Use a noisy image that doesn't compress well to force chunking
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, color.RGBA{uint8((x * 7 + y * 13) % 256), uint8((x*3 + y*11) % 256), uint8((x*17 + y*5) % 256), 255})
		}
	}
	tx, _, err := EncodeForKitty(img, 20, 10)
	if err != nil {
		t.Fatal(err)
	}

	// Large images should produce multiple chunks (m=1...m=0)
	if !strings.Contains(tx, ",m=1") {
		t.Error("large image should be chunked (missing m=1)")
	}
	if !strings.Contains(tx, "m=0") {
		t.Error("large image should have final chunk (missing m=0)")
	}
}

func TestEncodeForKitty_UniqueIDs(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))

	tx1, _, _ := EncodeForKitty(img, 1, 1)
	tx2, _, _ := EncodeForKitty(img, 1, 1)

	// Extract i= parameter
	extractID := func(s string) string {
		idx := strings.Index(s, "i=")
		if idx < 0 {
			return ""
		}
		end := strings.IndexAny(s[idx+2:], ",;")
		if end < 0 {
			return s[idx+2:]
		}
		return s[idx+2 : idx+2+end]
	}

	id1 := extractID(tx1)
	id2 := extractID(tx2)
	if id1 == id2 {
		t.Errorf("sequential images should have unique IDs, both got %s", id1)
	}
}

func TestDeleteAll(t *testing.T) {
	d := DeleteAll()
	if !strings.Contains(d, "\x1b_G") {
		t.Error("missing ESC_G")
	}
	if !strings.Contains(d, "a=d") {
		t.Error("missing delete action")
	}
}

func TestDiacriticsTable(t *testing.T) {
	// Must have enough entries for reasonable image heights
	if len(diacritics) < 20 {
		t.Errorf("diacritics table too small: %d entries (need at least 20)", len(diacritics))
	}

	// All entries should be valid combining characters (above U+0300)
	for i, d := range diacritics {
		if d < 0x0300 {
			t.Errorf("diacritics[%d] = U+%04X, expected combining character >= U+0300", i, d)
		}
	}

	// No duplicates
	seen := map[rune]int{}
	for i, d := range diacritics {
		if prev, ok := seen[d]; ok {
			t.Errorf("diacritics[%d] duplicates diacritics[%d] (U+%04X)", i, prev, d)
		}
		seen[d] = i
	}
}

func TestResizeToFit(t *testing.T) {
	t.Run("downscale", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 1280, 720))
		out := resizeToFit(img, 320, 160)
		b := out.Bounds()
		// 1280×720 scaled to fit 320×160 → limited by height: 284×160
		if b.Dx() > 320 || b.Dy() > 160 {
			t.Errorf("expected within 320×160, got %d×%d", b.Dx(), b.Dy())
		}
		if b.Dx() < 200 || b.Dy() < 100 {
			t.Errorf("downscaled too aggressively: %d×%d", b.Dx(), b.Dy())
		}
	})

	t.Run("no upscale", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 50, 30))
		out := resizeToFit(img, 320, 160)
		if out != img {
			t.Error("small image should be returned unchanged")
		}
	})

	t.Run("aspect ratio preserved", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 1280, 720))
		out := resizeToFit(img, 320, 160)
		b := out.Bounds()
		srcRatio := 1280.0 / 720.0
		dstRatio := float64(b.Dx()) / float64(b.Dy())
		if diff := srcRatio - dstRatio; diff > 0.05 || diff < -0.05 {
			t.Errorf("aspect ratio not preserved: src=%.2f dst=%.2f", srcRatio, dstRatio)
		}
	})
}

func TestEncodeForKitty_DownscalesLargeImage(t *testing.T) {
	// 1280×720 image should produce a transmitStr well under 200 KB
	// (without resize it would be ~2.8 MB).
	img := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	for y := 0; y < 720; y++ {
		for x := 0; x < 1280; x++ {
			img.SetRGBA(x, y, color.RGBA{
				uint8(x % 256), uint8(y % 256), 128, 255,
			})
		}
	}
	tx, _, err := EncodeForKitty(img, 20, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(tx) > 200_000 {
		t.Errorf("transmitStr too large after resize: %d bytes (want < 200000)", len(tx))
	}
}
