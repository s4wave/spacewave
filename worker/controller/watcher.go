package worker_controller

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_worker "github.com/aperturerobotics/forge/worker"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/aperturerobotics/identity"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ProcessState implements the state reconciliation loop.
func (c *Controller) ProcessState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	objKey := c.objKey
	if obj == nil {
		le.Debug("object does not exist, waiting")
		return true, nil
	}

	// unmarshal Worker state + build read cursor
	var workerState *forge_worker.Worker
	_, err = world.AccessObject(ctx, ws.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		var berr error
		workerState, berr = forge_worker.UnmarshalWorker(bcs)
		if berr != nil {
			return berr
		}
		return berr
	})
	if err != nil {
		return true, err
	}

	if err := workerState.Validate(); err != nil {
		le.WithError(err).Warn("object is invalid, waiting")
		return true, nil
	}

	// peerID may be empty here
	workerName, peerID := workerState.GetName(), c.peerID
	_ = workerName
	_ = peerID

	// lookup all keypair associated with the Worker.
	workerKeypairs, err := forge_worker.CollectWorkerKeypairs(ctx, ws, objKey)
	if err != nil {
		return true, err
	}

	// parse keypair peer ids
	workerPeerIDs := make([]peer.ID, len(workerKeypairs))
	for i, kp := range workerKeypairs {
		workerPeerIDs[i], err = kp.ParsePeerID()
		if err != nil {
			return true, errors.Wrapf(err, "keypairs[%d]", i)
		}
	}

	// if peer id is set, check that it matches any of the worker keypairs
	var peerIDPretty string
	if len(peerID) != 0 {
		peerIDPretty = peerID.Pretty()

		matched := -1
		for i, wPeerID := range workerPeerIDs {
			wPeerIDPretty := wPeerID.Pretty()
			if peerIDPretty == wPeerIDPretty {
				matched = i
				break
			}
		}
		if matched < 0 {
			le.Warnf(
				"worker %q: configured peer id does not match any of %d keypairs: %s",
				workerName,
				len(workerPeerIDs),
				peerIDPretty,
			)
			return true, nil
		}

		// override the list of peer ids with the configured
		workerPeerIDs = []peer.ID{peerID}
		workerKeypairs = []*identity.Keypair{workerKeypairs[matched]}
	}

	// push the current list of peer ids + keypairs + other info
	cState := newCState(workerState, workerPeerIDs, workerKeypairs)
	c.pushControllerState(cState)
	return true, nil
}

// _ is a type assertion
var _ world_control.ObjectLoopHandler = ((*Controller)(nil)).ProcessState
