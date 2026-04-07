package youtube

// Video represents a YouTube video.
type Video struct {
	ID              string
	Title           string
	ChannelName     string
	ChannelID       string
	Description     string
	DurationStr     string
	ViewCount       string
	LikeCount       string
	PublishedAt     string
	SubscriberCount string
	CommentsToken   string
	Thumbnails      []Thumbnail
	URL             string
}

// Channel represents a YouTube channel.
type Channel struct {
	ID              string
	Name            string
	Handle          string
	Description     string
	SubscriberCount string
	Thumbnails      []Thumbnail
	URL             string
}

// Comment represents a YouTube comment.
type Comment struct {
	ID          string
	AuthorName  string
	AuthorID    string
	Content     string
	LikeCount   string
	ReplyCount  int64
	PublishedAt string
	IsPinned    bool
	IsOwner     bool
	ReplyToken  string
}

// Playlist represents a YouTube playlist.
type Playlist struct {
	ID          string
	Title       string
	VideoCount  string
	ChannelName string
	Thumbnails  []Thumbnail
	URL         string
}

// Post represents a YouTube community post.
type Post struct {
	ID           string
	AuthorName   string
	AuthorID     string
	Content      string
	LikeCount    string
	PublishedAt  string
	Thumbnails   []Thumbnail
	DetailParams string // browseEndpoint params for FEpost_detail (to fetch comments)
}

// Thumbnail holds image URL and dimensions.
type Thumbnail struct {
	URL    string
	Width  int
	Height int
}

// Page is a paginated result set.
type Page[T any] struct {
	Items     []T
	NextToken string
	HasMore   bool
}
