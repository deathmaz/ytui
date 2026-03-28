package download

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Result holds the outcome of a download.
type Result struct {
	Title string
	Path  string
	Err   error
}

// Download runs yt-dlp to download a video. Blocks until complete.
func Download(url, format, outputDir, command string) Result {
	if outputDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Result{Err: fmt.Errorf("cannot determine home directory: %w", err)}
		}
		outputDir = filepath.Join(home, "Videos", "ytui")
	}
	outputDir = expandHome(outputDir)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return Result{Err: fmt.Errorf("create output dir: %w", err)}
	}

	tmpl := filepath.Join(outputDir, "%(title)s.%(ext)s")
	args := []string{
		"-o", tmpl,
		"--no-warnings",
	}
	if format != "" {
		args = append(args, "-f", format)
	}
	args = append(args, url)

	cmd := exec.Command(command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Result{Err: fmt.Errorf("yt-dlp: %w\n%s", err, output)}
	}

	path := extractOutputPath(string(output))
	title := extractTitle(path)
	if title == "" {
		title = "download complete"
	}

	return Result{Title: title, Path: path}
}

func extractOutputPath(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		// [download] Destination: /path/to/file.mp4
		if strings.Contains(line, "Destination:") {
			parts := strings.SplitN(line, "Destination:", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}

		// [Merger] Merging formats into "/path/to/file.mkv"
		if strings.Contains(line, "Merging formats into") {
			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			if start != -1 && end > start {
				return line[start+1 : end]
			}
		}

		// [download] /path/to/file.mp4 has already been downloaded
		if strings.Contains(line, "has already been downloaded") {
			parts := strings.SplitN(line, "]", 2)
			if len(parts) == 2 {
				return strings.TrimSuffix(strings.TrimSpace(parts[1]), " has already been downloaded")
			}
		}
	}
	return ""
}

func extractTitle(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != "" {
		return strings.TrimSuffix(base, ext)
	}
	return base
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
