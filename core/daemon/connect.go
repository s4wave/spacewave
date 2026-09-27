//go:build !js

package daemon

import "context"

// Connect resolves a native daemon target and completes Resource Init. An
// explicit socket is connect-only; otherwise it starts the current executable
// in detached serve mode when the state root has no daemon. The caller owns
// only the returned client, never the ready daemon.
func Connect(ctx context.Context, statePath, socketPath string) (*Client, error) {
	return NewConnector(nil, nil).Connect(ctx, statePath, socketPath)
}
