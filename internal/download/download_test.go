package download

import "testing"

func TestExtractOutputPath(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "destination line",
			output: "[download] Destination: /home/user/Videos/ytui/Go in 100 Seconds.mp4\n[download] 100%\n",
			want:   "/home/user/Videos/ytui/Go in 100 Seconds.mp4",
		},
		{
			name:   "merger line",
			output: "[download] 100%\n[Merger] Merging formats into \"/home/user/Videos/ytui/video.mkv\"\n",
			want:   "/home/user/Videos/ytui/video.mkv",
		},
		{
			name:   "already downloaded",
			output: "[download] /home/user/Videos/ytui/video.mp4 has already been downloaded\n",
			want:   "/home/user/Videos/ytui/video.mp4",
		},
		{
			name:   "no recognizable output",
			output: "some random output\n",
			want:   "",
		},
		{
			name:   "empty output",
			output: "",
			want:   "",
		},
		{
			name:   "destination with spaces in path",
			output: "[download] Destination: /home/user/My Videos/a video title.webm\n",
			want:   "/home/user/My Videos/a video title.webm",
		},
		{
			name:   "destination preferred over merger",
			output: "[download] Destination: /path/video.f137.mp4\n[download] Destination: /path/video.f251.webm\n[Merger] Merging formats into \"/path/video.mkv\"\n",
			want:   "/path/video.f137.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractOutputPath(tt.output)
			if got != tt.want {
				t.Errorf("extractOutputPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/Videos/Go in 100 Seconds.mp4", "Go in 100 Seconds"},
		{"/home/user/Videos/video.mkv", "video"},
		{"/home/user/Videos/noext", "noext"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractTitle(tt.path)
			if got != tt.want {
				t.Errorf("extractTitle(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	got := expandHome("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("expandHome should not modify absolute paths, got %q", got)
	}

	got = expandHome("relative/path")
	if got != "relative/path" {
		t.Errorf("expandHome should not modify relative paths, got %q", got)
	}

	// ~/something should expand (we can't assert the exact value since HOME varies)
	got = expandHome("~/test")
	if got == "~/test" {
		t.Error("expandHome should expand ~/test")
	}
}
