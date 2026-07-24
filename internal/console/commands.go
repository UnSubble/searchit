package console

// Command represents an interactive user control command.
type Command int

const (
	// CommandProgress triggers immediate progress rendering.
	CommandProgress Command = iota
	// CommandStats triggers extended statistics rendering.
	CommandStats
	// CommandStopTarget requests graceful shutdown of the current target.
	CommandStopTarget
	// CommandAbortAll requests immediate abort of all targets.
	CommandAbortAll

	// Backward-compatible aliases
	CommandStop  = CommandStopTarget
	CommandAbort = CommandAbortAll
)
