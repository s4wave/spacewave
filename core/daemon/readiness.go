//go:build !js

package daemon

import "os"

// readinessSuffix identifies the serve-owned notification beside the socket.
const readinessSuffix = ".ready"

// PublishReady wakes connectors after the daemon's listener and Resource
// service are ready and startup custody has transferred. Only the daemon
// holding the state lease may call it. Both native and distribution serve use
// this event, since listen itself produces no socket pathname event.
//
// The file is a notification, not evidence of a live daemon: connectors subscribe
// before dialing and always complete Resource Init. Rewriting it emits a fresh
// event on each startup, including when a previous daemon left the file behind.
func PublishReady(socketPath string) error {
	return os.WriteFile(socketPath+readinessSuffix, []byte("ready\n"), 0o600)
}
