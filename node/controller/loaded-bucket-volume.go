package node_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/util/keyed"
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
	// bh is the bucket handle
	// guarded by b.mtx
	bh volume.BucketHandle
}

// newLoadedBucketVolume constructs a new loaded bucket volume.
func (b *loadedBucket) newLoadedBucketVolume(volumeID string) (keyed.Routine, *loadedBucketVolume) {
	lbv := &loadedBucketVolume{
		b:        b,
		volumeID: volumeID,
		le:       b.le.WithField("volume-id", volumeID),
	}
	return lbv.execute, lbv
}

// execute executes the bucket volume tracker.
func (l *loadedBucketVolume) execute(ctx context.Context) error {
	_, diRef, err := l.b.c.b.AddDirective(
		volume.NewBuildBucketAPI(l.b.bucketID, l.volumeID),
		l,
	)
	if err != nil {
		return err
	}
	<-ctx.Done()
	diRef.Release()
	return nil
}

// HandleValueAdded is called when a value is added to the directive.
// Should not block.
func (l *loadedBucketVolume) HandleValueAdded(_ directive.Instance, av directive.AttachedValue) {
	val, ok := av.GetValue().(volume.BuildBucketAPIValue)
	if !ok {
		return
	}
	l.b.mtx.Lock()
	if lbv, exists := l.b.volumes.GetKey(l.volumeID); exists && lbv == l {
		if val.GetExists() {
			nbc := val.GetBucketConfig().CloneVT()
			if l.b.bucketConf == nil || nbc.GetRev() > l.b.bucketConf.GetRev() {
				l.le.
					WithField("bucket-rev", nbc.GetRev()).
					Debug("updated bucket config")
				l.b.bucketConf = nbc
			}
		} else {
			l.le.Debug("bucket not in volume")
		}
		if l.bh != val {
			l.bh = val
			l.b.bucketHandleSetDirty = true
		}
		l.b.wake.Broadcast()
	}
	l.b.mtx.Unlock()
}

// HandleValueRemoved is called when a value is removed from the directive.
// Should not block.
func (l *loadedBucketVolume) HandleValueRemoved(_ directive.Instance, av directive.AttachedValue) {
	val, ok := av.GetValue().(volume.BuildBucketAPIValue)
	if !ok || !val.GetExists() {
		return
	}
	l.b.mtx.Lock()
	if lbv, exists := l.b.volumes.GetKey(l.volumeID); exists && lbv == l {
		l.bh = nil
		l.b.bucketHandleSetDirty = true
		l.b.wake.Broadcast()
	}
	l.b.mtx.Unlock()
}

// HandleInstanceDisposed is called when a directive instance is disposed.
// This will occur if Close() is called on the directive instance.
func (l *loadedBucketVolume) HandleInstanceDisposed(_ directive.Instance) {
	l.b.mtx.Lock()
	existed, reset := l.b.volumes.ResetRoutine(l.volumeID, func(_ string, other *loadedBucketVolume) bool {
		return l == other
	})
	if existed && reset && l.bh != nil {
		l.bh = nil
		l.b.bucketHandleSetDirty = true
		l.b.wake.Broadcast()
	}
	l.b.mtx.Unlock()
}

// _ is a type assertion
var _ directive.ReferenceHandler = ((*loadedBucketVolume)(nil))
