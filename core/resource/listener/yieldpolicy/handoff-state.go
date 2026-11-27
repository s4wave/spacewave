package yieldpolicy

// HandoffState is the current runtime-handoff state emitted to the UI.
type HandoffState struct {
	// Active is true when the listener has yielded and is waiting for
	// the user to reclaim the runtime.
	Active bool
	// RequesterName is the name of the runtime that took over.
	RequesterName string
	// SocketPath is the socket path that was yielded.
	SocketPath string
	// SinceUnixMs is when the handoff started.
	SinceUnixMs int64
}
