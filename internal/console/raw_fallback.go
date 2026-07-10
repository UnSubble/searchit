//go:build !linux && !darwin && !windows

package console

// State holds the original terminal configuration (fallback).
type State struct{}

// MakeRaw is a stub fallback.
func MakeRaw(fd int) (*State, error) {
	return &State{}, nil
}

// Restore is a stub fallback.
func Restore(fd int, state *State) error {
	return nil
}
