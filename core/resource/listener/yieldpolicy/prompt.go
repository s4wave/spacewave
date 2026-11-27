package yieldpolicy

// Prompt is a pending takeover prompt.
type Prompt struct {
	// ID is the unique prompt identifier.
	ID string
	// RequesterName is the human-readable name of the requesting
	// runtime (e.g. "spacewave serve").
	RequesterName string
	// SocketPath is the Unix socket path the requester wants to bind.
	SocketPath string
	// DeadlineUnixMs is the auto-deny deadline in unix milliseconds.
	DeadlineUnixMs int64
}
