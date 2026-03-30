package player

import (
	"fmt"
	"os/exec"
	"syscall"
)

// Play launches the player as a detached background process.
// quality is a height like "1080", "720", "best", or "audio".
func Play(url, quality, playerCmd string, extraArgs []string) error {
	var args []string
	args = append(args, extraArgs...)

	switch quality {
	case "", "best":
		// Let mpv/yt-dlp pick the best
	case "audio":
		args = append(args, "--ytdl-format=bestaudio/best")
	default:
		// Use format-sort to prefer the selected resolution
		args = append(args, "--ytdl-raw-options=format-sort=res:"+quality)
	}

	args = append(args, url)

	cmd := exec.Command(playerCmd, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", playerCmd, err)
	}

	go cmd.Wait()
	return nil
}
