package console

// Command represents an interactive user control command.
type Command int

const (
	// CommandProgress triggers immediate progress rendering.
	CommandProgress Command = iota
	// CommandStats triggers extended statistics rendering.
	CommandStats
	// CommandStop requests graceful shutdown of the current target.
	CommandStop
	// CommandAbort requests immediate abort of all targets.
	CommandAbort
)
