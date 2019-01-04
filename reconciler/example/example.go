package reconciler_example

import (
	"context"
	"encoding/json"

	"github.com/aperturerobotics/hydra/bucket/event"
	"github.com/aperturerobotics/hydra/reconciler"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the Example volume controller.
const ControllerID = "hydra/reconciler/example/1"

// Version is the version of the reconciler example implementation.
var Version = semver.MustParse("0.0.1")

// Reconciler implements a basic example reconciler.
type Reconciler struct {
	// le is the logger
	le *logrus.Entry
}

// NewReconciler is the reconciler constructor.
func NewReconciler(
	le *logrus.Entry,
	conf *Config,
) reconciler.Reconciler {
	return &Reconciler{le: le}
}

// Execute executes the reconciler controller.
func (r *Reconciler) Execute(ctx context.Context, handle reconciler.Handle) error {
	r.le.Info("executing example reconciler")
	for {
		m, ok, err := handle.GetEventQueue().Peek()
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		e, err := bucket_event.UnmarshalBucketEvent(m.GetData())
		if err != nil {
			return err
		}
		dat, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if e.GetEventType() == bucket_event.EventType_EventType_PUT_BLOCK {
			br := e.GetPutBlock().GetBlockCommon().GetBlockRef()
			bh := handle.GetBucketHandle().GetBucket()
			dat, ok, err := bh.GetBlock(br)
			if err != nil {
				r.le.WithError(err).Warn("unable to lookup put block")
			} else {
				r.le.Debugf("lookup put block: found(%v) len(data)(%v)", ok, len(dat))
			}
		} else {
			r.le.Infof("read unknown reconciler event: %s", string(dat))
		}
		if err := handle.GetEventQueue().Ack(m.GetId()); err != nil {
			return err
		}
	}
}

// Close releases any resources used by the controller.
func (r *Reconciler) Close() error {
	return nil
}

// _ is a type assertion
var _ reconciler.Reconciler = ((*Reconciler)(nil))
