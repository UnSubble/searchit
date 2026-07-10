//go:build !linux && !darwin && !windows

package console

import "os"

// IsTerminal returns true if the given file descriptor is a character device (fallback).
func IsTerminal(fd uintptr) bool {
	stat, err := os.NewFile(fd, "").Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
