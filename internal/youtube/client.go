package youtube

import "context"

// Client defines the interface for YouTube data access.
type Client interface {
	// Search searches for videos by query.
	Search(ctx context.Context, query string, pageToken string) (*Page[Video], error)

	// GetVideo returns details for a single video.
	GetVideo(ctx context.Context, id string) (*Video, error)

	// GetComments returns comments for a video.
	GetComments(ctx context.Context, videoID string, pageToken string) (*Page[Comment], error)

	// GetReplies returns replies to a comment.
	GetReplies(ctx context.Context, commentID string, pageToken string) (*Page[Comment], error)

	// GetSubscriptions returns the authenticated user's subscriptions.
	GetSubscriptions(ctx context.Context, pageToken string) (*Page[Channel], error)

	// GetFeed returns the authenticated user's subscription feed.
	GetFeed(ctx context.Context, pageToken string) (*Page[Video], error)

	// GetChannelVideos returns videos from a channel.
	GetChannelVideos(ctx context.Context, channelID string, pageToken string) (*Page[Video], error)

	// GetChannelPlaylists returns playlists from a channel.
	GetChannelPlaylists(ctx context.Context, channelID string, pageToken string) (*Page[Playlist], error)

	// GetChannelPosts returns community posts from a channel.
	GetChannelPosts(ctx context.Context, channelID string, pageToken string) (*Page[Post], error)

	// GetChannelStreams returns livestreams from a channel.
	GetChannelStreams(ctx context.Context, channelID string, pageToken string) (*Page[Video], error)

	// GetPlaylistVideos returns videos in a playlist.
	GetPlaylistVideos(ctx context.Context, playlistID string, pageToken string) (*Page[Video], error)

	// GetPostComments returns comments on a community post.
	// On initial call, pass detailParams (from Post.DetailParams) with empty pageToken.
	// On continuation, pass the NextToken as pageToken (detailParams is ignored).
	GetPostComments(ctx context.Context, detailParams string, pageToken string) (*Page[Comment], error)

	// IsAuthenticated reports whether the client has valid credentials.
	IsAuthenticated() bool
}
