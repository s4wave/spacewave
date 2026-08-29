//go:build !js

package spacewave_cli

import "testing"

func TestForgeCreateWorkerExposesSessionPeerAndClusterInputs(t *testing.T) {
	cmd := newForgeCommand(nil)
	createWorker := findTestSubcommand(t, cmd, "create-worker")
	assertCommandFlags(t, createWorker, "state-path", "socket-path", "session-index", "space", "name", "peer-id", "cluster")
}
