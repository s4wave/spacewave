//go:build !js

package daemon

import "github.com/pkg/errors"

// ErrStarting means another process holds the state lease, so callers watch
// that process's socket.
var ErrStarting = errors.New("another daemon is starting")
