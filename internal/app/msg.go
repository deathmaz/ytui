package app

// View represents the active screen in the application.
type View int

const (
	ViewFeed View = iota
	ViewSubs
	ViewSearch
	ViewDynamicTab // a dynamic tab (video detail, channel, playlist, or post)
)
