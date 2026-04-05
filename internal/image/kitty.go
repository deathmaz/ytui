package image

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strings"
	"sync/atomic"
)

// Row/column diacritics from Kitty's specification.
// https://sw.kovidgoyal.net/kitty/_downloads/f0a0de9ec8d9ff4456206db8e0814937/rowcolumn-diacritics.txt
var diacritics = []rune{
	0x0305, 0x030D, 0x030E, 0x0310, 0x0312, 0x033D, 0x033E, 0x033F,
	0x0346, 0x034A, 0x034B, 0x034C, 0x0350, 0x0351, 0x0352, 0x0357,
	0x035B, 0x0363, 0x0364, 0x0365, 0x0366, 0x0367, 0x0368, 0x0369,
	0x036A, 0x036B, 0x036C, 0x036D, 0x036E, 0x036F, 0x0483, 0x0484,
	0x0485, 0x0486, 0x0487, 0x0592, 0x0593, 0x0594, 0x0595, 0x0597,
	0x0598, 0x0599, 0x059C, 0x059D, 0x059E, 0x059F, 0x05A0, 0x05A1,
	0x05A8, 0x05A9, 0x05AB, 0x05AC, 0x05AF, 0x05C4, 0x0610, 0x0611,
}

var nextImageID atomic.Uint32

func init() {
	nextImageID.Store(1)
}

// EncodeForKitty encodes an image for the Kitty Unicode placeholder protocol.
// Returns:
//   - transmitStr: escape sequences to transmit data + create virtual placement (use once)
//   - placeholderStr: U+10EEEE grid with diacritics (use in every View)
func EncodeForKitty(img image.Image, cols, rows int) (transmitStr, placeholderStr string, err error) {
	id := nextImageID.Add(1)

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return "", "", err
	}
	payload := base64.StdEncoding.EncodeToString(pngBuf.Bytes())

	var tx bytes.Buffer

	// Step 1: Transmit image data (a=T, don't display, q=2 quiet)
	const chunkSize = 4096
	for i := 0; i < len(payload); i += chunkSize {
		end := i + chunkSize
		if end > len(payload) {
			end = len(payload)
		}
		chunk := payload[i:end]
		isFirst := i == 0
		isLast := end == len(payload)

		tx.WriteString("\x1b_G")
		if isFirst {
			fmt.Fprintf(&tx, "a=t,q=2,i=%d,f=100", id)
			if !isLast {
				tx.WriteString(",m=1")
			}
		} else if isLast {
			tx.WriteString("m=0")
		} else {
			tx.WriteString("m=1")
		}
		tx.WriteByte(';')
		tx.WriteString(chunk)
		tx.WriteString("\x1b\\")
	}

	// Step 2: Create virtual placement (a=p, U=1 for unicode placeholders)
	fmt.Fprintf(&tx, "\x1b_Ga=p,U=1,i=%d,c=%d,r=%d,q=2\x1b\\", id, cols, rows)

	transmitStr = tx.String()

	// Step 3: Build placeholder grid with row diacritics
	placeholderStr = buildPlaceholders(id, cols, rows)

	return transmitStr, placeholderStr, nil
}

// buildPlaceholders creates the U+10EEEE grid with proper row diacritics.
// First cell of each row gets a row diacritic; remaining cells auto-inherit.
func buildPlaceholders(id uint32, cols, rows int) string {
	// Encode image ID as 24-bit color (R,G,B) to support >255 unique images.
	// 256-color mode (\033[38;5;Nm) only supports N=0-255, causing ID
	// collisions in sessions with many thumbnails.
	r := (id >> 16) & 0xFF
	g := (id >> 8) & 0xFF
	bl := id & 0xFF
	var b strings.Builder
	for row := 0; row < rows; row++ {
		fmt.Fprintf(&b, "\033[38;2;%d;%d;%dm", r, g, bl)
		for col := 0; col < cols; col++ {
			b.WriteRune('\U0010EEEE')
			if col == 0 && row < len(diacritics) {
				b.WriteRune(diacritics[row])
			}
		}
		b.WriteString("\033[39m")
		if row < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// DeleteAll returns an escape sequence that deletes all Kitty images.
func DeleteAll() string {
	return "\x1b_Ga=d,d=A,q=2\x1b\\"
}
