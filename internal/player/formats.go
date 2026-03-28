package player

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Format represents a video/audio format from yt-dlp.
type Format struct {
	ID         string
	Extension  string
	Resolution string
	Note       string
	Display    string // human-readable line
}

// FetchFormats runs yt-dlp -F to list available formats for a URL.
func FetchFormats(ctx context.Context, url string) ([]Format, error) {
	cmd := exec.CommandContext(ctx, "yt-dlp", "-F", "--no-warnings", url)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp -F: %w", err)
	}

	return parseFormats(string(out)), nil
}

func parseFormats(output string) []Format {
	var formats []Format
	scanner := bufio.NewScanner(strings.NewReader(output))
	headerPassed := false

	for scanner.Scan() {
		line := scanner.Text()

		// Skip until we pass the header separator
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "───") {
			headerPassed = true
			continue
		}
		if !headerPassed {
			continue
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		f := parseSingleFormat(line)
		if f.ID != "" {
			formats = append(formats, f)
		}
	}

	return formats
}

func parseSingleFormat(line string) Format {
	// yt-dlp format lines look like:
	// 251          webm       audio only audio_quality_medium  128k , webm_dash ...
	// 22           mp4        1280x720   30    ...
	// 137          mp4        1920x1080  30    ...
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return Format{}
	}

	f := Format{
		ID:        fields[0],
		Extension: fields[1],
		Display:   line,
	}

	// Try to find resolution
	for _, field := range fields[2:] {
		if strings.Contains(field, "x") && !strings.Contains(field, "http") {
			f.Resolution = field
			break
		}
		if field == "audio" {
			f.Resolution = "audio only"
			break
		}
	}

	// Build note from remaining fields
	if len(fields) > 3 {
		f.Note = strings.Join(fields[2:], " ")
	}

	return f
}

// CommonFormats returns a simplified list of common quality presets.
func CommonFormats() []Format {
	return []Format{
		{ID: "bestvideo[height<=2160]+bestaudio/best", Resolution: "2160p", Display: "Best (up to 4K)"},
		{ID: "bestvideo[height<=1080]+bestaudio/best", Resolution: "1080p", Display: "1080p (Full HD)"},
		{ID: "bestvideo[height<=720]+bestaudio/best", Resolution: "720p", Display: "720p (HD)"},
		{ID: "bestvideo[height<=480]+bestaudio/best", Resolution: "480p", Display: "480p"},
		{ID: "bestaudio/best", Resolution: "audio", Display: "Audio only (best)"},
	}
}
