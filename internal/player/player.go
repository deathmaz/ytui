package player

import (
	"fmt"
	"os/exec"
	"syscall"
)

// Play launches the player as a detached background process.
// extraArgs are additional arguments from the config file.
func Play(url, ytdlFormat, playerCmd string, extraArgs []string) error {
	var args []string
	args = append(args, extraArgs...)
	if ytdlFormat != "" {
		args = append(args, "--ytdl-format="+ytdlFormat)
	}
	args = append(args, url)

	cmd := exec.Command(playerCmd, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", playerCmd, err)
	}

	// Let the process run independently
	go cmd.Wait()

	return nil
}
