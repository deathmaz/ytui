package app

import (
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// emulateGrid feeds raw terminal output bytes through a minimal VT100-ish
// emulator and returns the resulting cell grid as plain text. Golden files
// store this plain-text grid instead of the raw byte stream, which is
// inherently flaky under v2's diff-based Cursed Renderer: the same logical
// screen can be produced by many different byte streams (one-shot redraw
// vs. several partial diffs), and the renderer's burst timing varies
// enough between runs to make byte-exact snapshots unreliable.
//
// Supported sequences: CUP (ESC[H ESC[r;cH ESC[r;cf), HPA (ESC[n`), VPA
// (ESC[nd), EL (ESC[nK — line clear), ECH (ESC[nX — erase chars), ED
// (ESC[2J), IL (ESC[nL — insert line), REP (ESC[nb — repeat last), CR,
// LF. Other sequences (SGR, bracketed-paste, alt-screen, etc.) are
// dropped so only cell content is compared.
func emulateGrid(data []byte, cols, rows int) string {
	grid := make([][]rune, rows)
	for i := range grid {
		grid[i] = make([]rune, cols)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}
	r, c := 0, 0
	var lastRune rune = ' '
	p := ansi.GetParser()
	defer ansi.PutParser(p)
	var state byte
	for len(data) > 0 {
		seq, _, n, newState := ansi.DecodeSequence(data, state, p)
		state = newState
		data = data[n:]
		s := string(seq)
		if len(s) == 0 {
			continue
		}
		// Handle control chars.
		if s == "\r" {
			c = 0
			continue
		}
		if s == "\n" {
			// Terminal in cooked mode (our default — mapNl=true) maps LF
			// to CRLF on output. Match that behavior.
			c = 0
			if r < rows-1 {
				r++
			}
			continue
		}
		if s == "\b" {
			if c > 0 {
				c--
			}
			continue
		}
		// CSI sequences.
		if ansi.HasCsiPrefix(seq) {
			cmd := p.Command()
			n0 := paramOrParser(p, 0, 1)
			n1 := paramOrParser(p, 1, 1)
			switch cmd {
			case 'H', 'f': // CUP
				r = clamp(paramOrParser(p, 0, 1)-1, 0, rows-1)
				c = clamp(paramOrParser(p, 1, 1)-1, 0, cols-1)
			case '`': // HPA
				c = clamp(n0-1, 0, cols-1)
			case 'd': // VPA
				r = clamp(n0-1, 0, rows-1)
			case 'A': // CUU
				r = clamp(r-n0, 0, rows-1)
			case 'B': // CUD
				r = clamp(r+n0, 0, rows-1)
			case 'C': // CUF
				c = clamp(c+n0, 0, cols-1)
			case 'D': // CUB
				c = clamp(c-n0, 0, cols-1)
			case 'K': // EL
				ki := paramOrParser(p, 0, 0)
				switch ki {
				case 0:
					for k := c; k < cols; k++ {
						grid[r][k] = ' '
					}
				case 1:
					for k := 0; k <= c && k < cols; k++ {
						grid[r][k] = ' '
					}
				case 2:
					for k := 0; k < cols; k++ {
						grid[r][k] = ' '
					}
				}
			case 'J': // ED
				ji := paramOrParser(p, 0, 0)
				if ji == 2 {
					for i := 0; i < rows; i++ {
						for j := 0; j < cols; j++ {
							grid[i][j] = ' '
						}
					}
				}
			case 'X': // ECH
				for k := c; k < c+n0 && k < cols; k++ {
					grid[r][k] = ' '
				}
			case 'L': // IL — insert line; leave contents alone.
				_ = n0
			case 'b': // REP — repeat previous grapheme.
				for i := 0; i < n0 && c < cols; i++ {
					grid[r][c] = lastRune
					c++
				}
			}
			_ = n1
			continue
		}
		// APC / OSC / DCS / PM — drop.
		if ansi.HasApcPrefix(seq) || ansi.HasOscPrefix(seq) ||
			ansi.HasDcsPrefix(seq) || ansi.HasPmPrefix(seq) {
			continue
		}
		// ESC (plain) — drop.
		if ansi.HasEscPrefix(seq) {
			continue
		}
		// Printable grapheme cluster — write at cursor.
		ru, _ := utf8.DecodeRuneInString(s)
		if ru == utf8.RuneError {
			continue
		}
		if c < cols && r < rows {
			grid[r][c] = ru
			lastRune = ru
			c++
		}
	}
	// Serialize grid to plain text with trailing whitespace stripped.
	var out []byte
	for i, row := range grid {
		end := len(row)
		for end > 0 && row[end-1] == ' ' {
			end--
		}
		for _, ru := range row[:end] {
			out = utf8.AppendRune(out, ru)
		}
		if i < rows-1 {
			out = append(out, '\n')
		}
	}
	return string(out)
}

func paramOrParser(p *ansi.Parser, i, def int) int {
	v, _ := p.Param(i, def)
	return v
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
