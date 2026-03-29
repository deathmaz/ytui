package shared

import "github.com/deathmaz/ytui/internal/youtube"

// VideoSelectedMsg is emitted when a user selects a video from any list view.
type VideoSelectedMsg struct {
	Video youtube.Video
}
