package temporal

// Signal names for external interaction with review workflows.
// These constants are used to send signals to running Temporal workflows.
const (
	// SignalPause requests workflow pause at next checkpoint.
	SignalPause = "pause"

	// SignalResume requests workflow resume from paused state.
	SignalResume = "resume"

	// SignalStop requests graceful workflow stop with partial results.
	SignalStop = "stop"
)
