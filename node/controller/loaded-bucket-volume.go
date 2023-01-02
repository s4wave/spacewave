package node_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/sirupsen/logrus"
)

// loadedBucketVolume contains state for a loaded bucket volume controller.
type loadedBucketVolume struct {
	// b is constant
	b *loadedBucket
	// volumeID is constant
	volumeID string
	// le is constant after init
	le *logrus.Entry
	// ref is the reference to the directive
	ref directive.Reference
	// bh is the bucket handle
	// guarded by b.mtx
	bh volume.BucketHandle
}

// init initializes the volume
func (l *loadedBucketVolume) init(ctx context.Context) error {
	l.le = l.b.le.WithField("volume-id", l.volumeID)
	_, diRef, err := l.b.c.b.AddDirective(
		volume.NewBuildBucketAPI(l.b.bucketID, l.volumeID),
		l,
	)
	if err != nil {
		return err
	}
	l.ref = diRef
	return nil
}

// HandleValueAdded is called when a value is added to the directive.
// Should not block.
func (l *loadedBucketVolume) HandleValueAdded(_ directive.Instance, av directive.AttachedValue) {
	val, ok := av.GetValue().(volume.BuildBucketAPIValue)
	if !ok {
		return
	}
	go func() { // avoid mutex conflict here
		if !val.GetExists() {
			l.le.Debug("bucket not in volume")
			l.b.ClearVolume(l.volumeID)
			return
		}
		l.b.mtx.Lock()
		if lbv := l.b.volumes[l.volumeID]; lbv == l {
			nbc := val.GetBucketConfig()
			if nbc != nil {
				if l.b.bucketConf == nil || l.b.bucketConf.GetVersion() < nbc.GetVersion() {
					l.le.
						WithField("bucket-revision", nbc.GetVersion()).
						Debug("got latest/newer bucket config")
					l.b.bucketConf = nbc
				}
			}
			l.bh = val
			l.b.bucketHandleSetDirty = true
			defer l.b.wake()
		}
		l.b.mtx.Unlock()
	}()
}

// HandleValueRemoved is called when a value is removed from the directive.
// Should not block.
func (l *loadedBucketVolume) HandleValueRemoved(_ directive.Instance, av directive.AttachedValue) {
	val, ok := av.GetValue().(volume.BuildBucketAPIValue)
	if !ok || !val.GetExists() {
		return
	}
	go func() {
		l.b.mtx.Lock()
		if l.bh == val {
			if lbv := l.b.volumes[l.volumeID]; lbv == l {
				l.bh = nil
				l.b.bucketHandleSetDirty = true
				defer l.b.wake()
			}
		}
		l.b.mtx.Unlock()
	}()
}

// HandleInstanceDisposed is called when a directive instance is disposed.
// This will occur if Close() is called on the directive instance.
func (l *loadedBucketVolume) HandleInstanceDisposed(_ directive.Instance) {
	go func() {
		l.ref.Release()
		l.b.mtx.Lock()
		if lbv := l.b.volumes[l.volumeID]; lbv == l {
			delete(l.b.volumes, l.volumeID)
			if l.bh != nil {
				l.bh = nil
				l.b.bucketHandleSetDirty = true
				defer l.b.wake()
			}
		}
		l.b.mtx.Unlock()
	}()
}

// _ is a type assertion
var _ directive.ReferenceHandler = ((*loadedBucketVolume)(nil))
