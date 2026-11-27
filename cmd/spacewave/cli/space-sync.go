//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"strconv"

	"github.com/s4wave/spacewave/core/sobject"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// readSpaceHealth returns the Space's current health, or nil when the daemon
// cannot report it.
func readSpaceHealth(ctx context.Context, sess *s4wave_session.Session, spaceID string) *sobject.SharedObjectHealth {
	// Read the first health snapshot and stop watching.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	strm, err := sess.WatchSharedObjectHealth(ctx, spaceID)
	if err != nil {
		return nil
	}
	resp, err := strm.Recv()
	if err != nil {
		return nil
	}
	return resp.GetHealth()
}

// formatSyncPeerCount summarizes how many participants have a running stream.
func formatSyncPeerCount(peers []*sobject.SOSyncPeer) string {
	// Report the absence of peers, which a Space on one device always
	// reports.
	if len(peers) == 0 {
		return "no peers since start"
	}

	// Count the connected peers.
	var connected int
	for _, p := range peers {
		if p.GetConnected() {
			connected++
		}
	}
	return strconv.Itoa(connected) + " of " + strconv.Itoa(len(peers)) + " peers connected"
}

// writeSyncPeers writes one row per participant stream: its state, when the
// state began, and the error that ended a disconnected stream.
func writeSyncPeers(w io.Writer, peers []*sobject.SOSyncPeer) {
	// Write nothing without peers, and let the summary line say so.
	if len(peers) == 0 {
		return
	}

	// Write a row for each peer.
	rows := [][]string{{"PEER", "STATE", "SINCE", "ERROR"}}
	for _, p := range peers {
		state := "disconnected"
		if p.GetConnected() {
			state = "connected"
		}
		rows = append(rows, []string{
			p.GetPeerId(),
			state,
			formatTimestamp(p.GetSince().AsTime().Unix()),
			p.GetError(),
		})
	}
	io.WriteString(w, "\nSync peers\n")
	writeTable(w, "  ", rows)
}
