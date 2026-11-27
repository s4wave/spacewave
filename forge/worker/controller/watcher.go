package worker_controller

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	forge_world "github.com/s4wave/spacewave/forge/world"
	"github.com/s4wave/spacewave/net/peer"
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
	// Wait for the Worker object before reconciling its assignments.
	objKey := c.objKey
	if obj == nil {
		le.Debug("object does not exist, waiting")
		return true, nil
	}

	// unmarshal Worker state + build read cursor
	var workerState *forge_worker.Worker
	_, err = world.AccessObject(ctx, ws.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		var berr error
		workerState, berr = forge_worker.UnmarshalWorker(ctx, bcs)
		return berr
	})
	if err != nil {
		return true, err
	}

	// Require valid Worker state before starting its trackers.
	if err := workerState.Validate(); err != nil {
		le.WithError(err).Warn("object is invalid, waiting")
		return true, nil
	}

	// peerID may be empty here
	workerName, peerID := workerState.GetName(), c.peerID

	// lookup all keypair associated with the Worker.
	workerKeypairs, workerKeypairKeys, err := forge_worker.CollectWorkerKeypairs(ctx, ws, objKey)
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
	var peerIDString string
	if len(peerID) != 0 {
		peerIDString = peerID.String()

		matched := -1
		for i, wPeerID := range workerPeerIDs {
			wPeerIDString := wPeerID.String()
			if peerIDString == wPeerIDString {
				matched = i
				break
			}
		}
		if matched < 0 {
			le.Warnf(
				"worker %q: configured peer id does not match any of %d keypairs: %s",
				workerName,
				len(workerPeerIDs),
				peerIDString,
			)
			return true, nil
		}

		// Override the list of peer ids with the configured peer id.
		// workerPeerIDs = []peer.ID{peerID}
		// workerKeypairs = []*identity.Keypair{workerKeypairs[matched]}
		workerKeypairKeys = []string{workerKeypairKeys[matched]}
	}

	// Look up the objects assigned to the keypairs. The read set records the
	// keypair bodies and their assignment edges, so this handler runs again
	// only when the Worker, its keypairs or their assignments change.
	assignedObjs, err := forge_world.ListKeypairObjects(ctx, ws, workerKeypairKeys...)
	if err != nil {
		return true, err
	}

	// Reconcile object trackers with the assignments, logging what changed.
	added, removed := c.objectTrackers.SyncKeys(assignedObjs, true)
	le = le.WithField("assigned", len(assignedObjs))
	if len(added) == 0 && len(removed) == 0 {
		le.Debug("worker assignments unchanged")
		return true, nil
	}
	le.Infof("worker assignments changed: added %v, removed %v", added, removed)
	return true, nil
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = (*Controller)(nil).ProcessState
