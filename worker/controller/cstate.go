package worker_controller

import (
	forge_worker "github.com/aperturerobotics/forge/worker"
	"github.com/aperturerobotics/identity"
	"github.com/libp2p/go-libp2p-core/peer"
)

// cState is the current controller state config pushed to the Execute loop.
type cState struct {
	// worker is the worker object
	worker *forge_worker.Worker
	// peerIDs is the list of peer ids to service
	peerIDs []peer.ID
	// keypairs is the list of keypairs corresponding to peerIDs
	// len(keypairs) must match len(peerIDs)
	keypairs []*identity.Keypair
}

// newCState constructs the cstate
func newCState(w *forge_worker.Worker, peerIDs []peer.ID, kps []*identity.Keypair) *cState {
	return &cState{
		worker:   w,
		peerIDs:  peerIDs,
		keypairs: kps,
	}
}
