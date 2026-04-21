package reconciler_example

import (
	"context"

	"github.com/blang/semver/v4"
	bucket_event "github.com/s4wave/spacewave/db/bucket/event"
	"github.com/s4wave/spacewave/db/reconciler"
	"github.com/sirupsen/logrus"
)

// ControllerID is the id for the reconciler example implementation.
const ControllerID = "hydra/reconciler/example"

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
		m, ok, err := handle.GetEventQueue().Peek(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		e := &bucket_event.Event{}
		if err := e.UnmarshalVT(m.GetData()); err != nil {
			return err
		}
		dat, err := e.MarshalJSON()
		if err != nil {
			return err
		}
		if e.GetEventType() == bucket_event.EventType_EventType_PUT_BLOCK {
			br := e.GetPutBlock().GetBlockCommon().GetBlockRef()
			bh := handle.GetBucketHandle().GetBucket()
			dat, ok, err := bh.GetBlock(ctx, br)
			if err != nil {
				r.le.WithError(err).Warn("unable to lookup put block")
			} else {
				r.le.Debugf("lookup put block: found(%v) len(data)(%v)", ok, len(dat))
			}
		} else {
			r.le.Infof("read unknown reconciler event: %s", string(dat))
		}
		if err := handle.GetEventQueue().Ack(ctx, m.GetId()); err != nil {
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
