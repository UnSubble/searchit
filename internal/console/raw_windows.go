//go:build windows

package console

import "golang.org/x/sys/windows"

// State holds the original terminal configuration.
type State struct {
	mode uint32
}

// MakeRaw puts the terminal in raw mode.
func MakeRaw(fd int) (*State, error) {
	var mode uint32
	err := windows.GetConsoleMode(windows.Handle(fd), &mode)
	if err != nil {
		return nil, err
	}

	rawMode := mode &^ (uint32(windows.ENABLE_LINE_INPUT) | uint32(windows.ENABLE_ECHO_INPUT) | uint32(windows.ENABLE_PROCESSED_INPUT))
	err = windows.SetConsoleMode(windows.Handle(fd), rawMode)
	if err != nil {
		return nil, err
	}

	return &State{mode: mode}, nil
}

// Restore restores the original terminal state.
func Restore(fd int, state *State) error {
	if state == nil {
		return nil
	}
	return windows.SetConsoleMode(windows.Handle(fd), state.mode)
}
