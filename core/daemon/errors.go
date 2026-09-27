//go:build !js

package daemon

import "github.com/pkg/errors"

// ErrStarting means another process holds the state lease; callers watch its socket.
var ErrStarting = errors.New("another daemon is starting")
