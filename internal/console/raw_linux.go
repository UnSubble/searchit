//go:build linux

package console

import (
	"syscall"
	"unsafe"
)

// State holds the original terminal configuration.
type State struct {
	termios syscall.Termios
}

// MakeRaw puts the terminal in raw mode.
func MakeRaw(fd int) (*State, error) {
	var oldState syscall.Termios
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&oldState)), 0, 0, 0)
	if err != 0 {
		return nil, err
	}

	newState := oldState
	newState.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	newState.Oflag &^= syscall.OPOST
	newState.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	newState.Cflag &^= syscall.CSIZE | syscall.PARENB
	newState.Cflag |= syscall.CS8
	newState.Cc[syscall.VMIN] = 1
	newState.Cc[syscall.VTIME] = 0

	_, _, err = syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&newState)), 0, 0, 0)
	if err != 0 {
		return nil, err
	}

	return &State{termios: oldState}, nil
}

// Restore restores the original terminal state.
func Restore(fd int, state *State) error {
	if state == nil {
		return nil
	}
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&state.termios)), 0, 0, 0)
	if err != 0 {
		return err
	}
	return nil
}
