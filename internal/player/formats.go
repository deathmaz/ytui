package player

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Format represents a quality option for video playback.
type Format struct {
	ID      string // used internally for mpv format-sort
	Display string // shown to user in picker
}

// FetchFormats runs yt-dlp -F to discover available resolutions for a URL.
// Returns deduplicated quality options sorted from highest to lowest.
func FetchFormats(ctx context.Context, url, command string) ([]Format, error) {
	cmd := exec.CommandContext(ctx, command, "-F", "--no-warnings", url)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s -F: %w", command, err)
	}

	resolutions := parseResolutions(string(out))
	if len(resolutions) == 0 {
		return DefaultFormats(), nil
	}

	var formats []Format
	formats = append(formats, Format{ID: "best", Display: "Best available"})
	for _, h := range resolutions {
		formats = append(formats, Format{
			ID:      fmt.Sprintf("%d", h),
			Display: fmt.Sprintf("%dp", h),
		})
	}
	formats = append(formats, Format{ID: "audio", Display: "Audio only"})
	return formats, nil
}

// parseResolutions extracts unique video heights from yt-dlp -F output.
// Returns sorted descending (e.g., [2160, 1080, 720, 480, 360]).
func parseResolutions(output string) []int {
	seen := map[int]bool{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	headerPassed := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "───") {
			headerPassed = true
			continue
		}
		if !headerPassed {
			continue
		}

		// Look for resolution like "1280x720" or "1920x1080"
		for _, field := range strings.Fields(line) {
			if strings.Contains(field, "x") && !strings.Contains(field, "http") {
				parts := strings.Split(field, "x")
				if len(parts) == 2 {
					if h, err := strconv.Atoi(parts[1]); err == nil && h > 0 {
						seen[h] = true
					}
				}
			}
		}
	}

	var heights []int
	for h := range seen {
		heights = append(heights, h)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(heights)))
	return heights
}

// DefaultFormats returns fallback quality presets when yt-dlp is unavailable.
func DefaultFormats() []Format {
	return []Format{
		{ID: "best", Display: "Best available"},
		{ID: "1080", Display: "1080p"},
		{ID: "720", Display: "720p"},
		{ID: "480", Display: "480p"},
		{ID: "audio", Display: "Audio only"},
	}
}
