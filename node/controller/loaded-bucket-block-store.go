package node_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/util/keyed"
	"github.com/sirupsen/logrus"
)

// loadedBucketBlockStore contains state for a block store with a loaded bucket.
type loadedBucketBlockStore struct {
	// b is constant
	b *loadedBucket
	// blockStoreID is constant
	blockStoreID string
	// le is constant after init
	le *logrus.Entry
	// bh is the bucket handle
	// guarded by b.mtx
	bh bucket.BucketHandle
}

// newLoadedBucketBlockStore constructs a new loaded bucket block store.
func (b *loadedBucket) newLoadedBucketBlockStore(blockStoreID string) (keyed.Routine, *loadedBucketBlockStore) {
	lbv := &loadedBucketBlockStore{
		b:            b,
		blockStoreID: blockStoreID,
		le:           b.le.WithField("block-store-id", blockStoreID),
	}
	return lbv.execute, lbv
}

// execute executes the bucket block store tracker.
func (l *loadedBucketBlockStore) execute(ctx context.Context) error {
	_, diRef, err := l.b.c.b.AddDirective(
		bucket.NewBuildBucketAPI(l.b.bucketID, l.blockStoreID),
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
func (l *loadedBucketBlockStore) HandleValueAdded(_ directive.Instance, av directive.AttachedValue) {
	val, ok := av.GetValue().(bucket.BuildBucketAPIValue)
	if !ok {
		return
	}
	l.b.mtx.Lock()
	if lbv, exists := l.b.blockStores.GetKey(l.blockStoreID); exists && lbv == l {
		if val.GetExists() {
			nbc := val.GetBucketConfig().CloneVT()
			if l.b.bucketConf == nil || nbc.GetRev() > l.b.bucketConf.GetRev() {
				l.le.
					WithField("bucket-rev", nbc.GetRev()).
					Debug("updated bucket config")
				l.b.bucketConf = nbc
			}
		} else {
			l.le.Debug("bucket not in block store")
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
func (l *loadedBucketBlockStore) HandleValueRemoved(_ directive.Instance, av directive.AttachedValue) {
	val, ok := av.GetValue().(bucket.BuildBucketAPIValue)
	if !ok || !val.GetExists() {
		return
	}
	l.b.mtx.Lock()
	if lbv, exists := l.b.blockStores.GetKey(l.blockStoreID); exists && lbv == l {
		l.bh = nil
		l.b.bucketHandleSetDirty = true
		l.b.wake.Broadcast()
	}
	l.b.mtx.Unlock()
}

// HandleInstanceDisposed is called when a directive instance is disposed.
// This will occur if Close() is called on the directive instance.
func (l *loadedBucketBlockStore) HandleInstanceDisposed(_ directive.Instance) {
	l.b.mtx.Lock()
	existed, reset := l.b.blockStores.ResetRoutine(l.blockStoreID, func(_ string, other *loadedBucketBlockStore) bool {
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
var _ directive.ReferenceHandler = ((*loadedBucketBlockStore)(nil))
