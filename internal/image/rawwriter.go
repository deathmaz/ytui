package image

import (
	"io"
	"os"
	"sync"
)

// Ultraviolet's StyledString parser (used by Bubble Tea v2's Cursed Renderer)
// drops APC sequences — cell.Content is overwritten when the next printable
// rune arrives, discarding any accumulated Kitty graphics escapes. Embedding
// Kitty APC sequences in the View string therefore silently fails in v2.
//
// Workaround: stream APC bytes directly to stdout through a writer shared
// with the renderer. A single mutex serializes renderer frame writes and our
// graphics writes so byte streams don't interleave mid-sequence (which would
// land inside a CSI/APC sequence and corrupt both).

var (
	stdoutMu sync.Mutex
	stdoutW  io.Writer = os.Stdout
)

// SetRawOutput swaps the writer used by RawWrite. Intended for tests that
// need to capture the APC byte stream.
func SetRawOutput(w io.Writer) {
	stdoutMu.Lock()
	stdoutW = w
	stdoutMu.Unlock()
}

// SyncedStdout wraps os.Stdout with a mutex shared with RawWrite. It
// satisfies the term.File interface (io.ReadWriteCloser + Fd) so Bubble
// Tea's TTY detection still works — pass it via tea.WithOutput and the
// renderer's frame writes will serialize with our Kitty transmit writes.
type SyncedStdout struct{}

func (SyncedStdout) Read(p []byte) (int, error) { return os.Stdin.Read(p) }

func (SyncedStdout) Write(p []byte) (int, error) {
	stdoutMu.Lock()
	defer stdoutMu.Unlock()
	return os.Stdout.Write(p)
}

func (SyncedStdout) Close() error { return nil }

func (SyncedStdout) Fd() uintptr { return os.Stdout.Fd() }

// RawWrite emits raw bytes directly to stdout, serialized with any writer
// that shares stdoutMu (e.g. SyncedStdout wired into tea.WithOutput). Use
// for Kitty image APC sequences that the v2 renderer's cell parser would
// otherwise drop from View content.
func RawWrite(s string) {
	if s == "" {
		return
	}
	stdoutMu.Lock()
	_, _ = stdoutW.Write([]byte(s))
	stdoutMu.Unlock()
}
