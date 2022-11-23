package worker_controller

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/aperturerobotics/util/keyed"
	"github.com/sirupsen/logrus"
)

// keypairTracker tracks a Keypair linked to the Worker.
//
// the keypair rev is incremented when a new object is linked
// this notifies the worker watcher to re-scan for objects to track
type keypairTracker struct {
	// c is the controller
	c *Controller
	// objKey is the object key
	objKey string

	// objLoop is the object tracking loop
	objLoop *world_control.ObjectLoop
	// the following fields are managed by processState
	lastRev uint64
}

// newKeypairTracker constructs a new worker keypair tracker routine.
func (c *Controller) newKeypairTracker(key string) (keyed.Routine, *keypairTracker) {
	tr := &keypairTracker{
		c:      c,
		objKey: key,
	}
	tr.objLoop = world_control.NewObjectLoop(
		c.le.WithField("object-loop", "keypair-tracker"),
		key,
		tr.processState,
	)
	return tr.execute, tr
}

// execute executes the job tracker.
func (t *keypairTracker) execute(ctx context.Context) error {
	objKey, le := t.objKey, t.c.le

	le.Debugf("starting keypair tracker: %s", objKey)
	return world_control.ExecuteBusObjectLoop(
		ctx,
		t.c.bus,
		t.c.conf.GetEngineId(),
		true, t.objLoop,
	)
}

// processState processes the state for the job.
func (t *keypairTracker) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	// wake the worker if the revision changes
	lastRev := t.lastRev
	if lastRev != 0 && lastRev < rev {
		t.c.Wake()
	}
	t.lastRev = rev
	return true, nil
}

// _ is a type assertion
var _ world_control.ObjectLoopHandler = ((*keypairTracker)(nil)).processState
