package node_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/sirupsen/logrus"
)

// loadedBucket contains state for a loaded bucket.
type loadedBucket struct {
	c        *Controller
	le       *logrus.Entry
	bucketID string

	stateCtr  *ccontainer.CContainer[*loadedBucketState]
	lookupCtr *ccontainer.CContainer[bucket_lookup.Lookup]

	bcast                 broadcast.Broadcast
	lastState             *loadedBucketState
	bucketConf            *bucket.Config
	lookupCtrlRef         bucket_lookup.Controller
	blockStores           *keyed.Keyed[string, *loadedBucketBlockStore]
	bucketHandleSetPushed bool
	bucketHandleSetDirty  bool
}

// loadedBucketState is emitted when the state of the bucket changes.
type loadedBucketState struct {
	// disposed indicates this loadedBucket instance is disposed.
	disposed bool
	// info contains the latest bucket information
	info *bucket.BucketInfo
}

// clone copies the state.
func (l *loadedBucketState) clone() *loadedBucketState {
	if l == nil {
		return nil
	}
	return &loadedBucketState{
		disposed: l.disposed,
		info:     l.info.CloneVT(),
	}
}

// equal compares the two states.
func (l *loadedBucketState) equal(ot *loadedBucketState) bool {
	if (ot == nil) != (l == nil) {
		return false
	}
	if ot == nil {
		return true
	}
	return l.disposed == ot.disposed && l.info.EqualVT(ot.info)
}

// newLoadedBucket constructs a new loaded bucket.
func (c *Controller) newLoadedBucket(bucketID string) (keyed.Routine, *loadedBucket) {
	lb := &loadedBucket{
		c:         c,
		le:        c.le.WithField("bucket-id", bucketID),
		bucketID:  bucketID,
		lookupCtr: ccontainer.NewCContainer[bucket_lookup.Lookup](nil),
		stateCtr:  ccontainer.NewCContainer[*loadedBucketState](nil),
	}
	lb.blockStores = keyed.NewKeyed(lb.newLoadedBucketBlockStore)
	return lb.execute, lb
}

// execute executes the loaded bucket routine.
func (b *loadedBucket) execute(ctx context.Context) error {
	b.le.Debug("starting bucket tracking")

	// State management routines.
	defer b.blockStores.SyncKeys(nil, false)
	defer b.le.Debug("exited bucket tracking")

	var st loadedBucketState
	emitState := func() {
		if b.lastState.equal(&st) {
			return
		}
		b.lastState = (&st).clone()
		b.stateCtr.SetValue(b.lastState)
	}
	defer func() {
		st.disposed = true
		emitState()
	}()

	// startup
	var waitCh <-chan struct{}
	b.blockStores.SetContext(ctx, true)

	var lookupCtrCancel context.CancelFunc
	for {
		var stDirty bool

		if waitCh != nil {
			select {
			case <-ctx.Done():
				if lookupCtrCancel != nil {
					lookupCtrCancel()
				}
				return ctx.Err()
			case <-waitCh:
			}
		}

		b.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			if !st.info.GetConfig().EqualVT(b.bucketConf) {
				stDirty = true
				st.info = bucket.NewBucketInfo(b.bucketConf)
				if lookupCtrCancel != nil {
					b.lookupCtrlRef = nil
					lookupCtrCancel()
					lookupCtrCancel = nil
					b.clearLookup()
				}
			}

			if b.bucketHandleSetDirty && b.lookupCtrlRef != nil {
				vols := b.blockStores.GetKeysWithData()
				handles := make([]bucket.BucketHandle, 0, len(vols))
				for _, vdat := range vols {
					v := vdat.Data
					if v != nil && v.bh != nil && v.bh.GetExists() {
						handles = append(handles, v.bh)
					}
				}
				if len(handles) != 0 || b.bucketHandleSetPushed {
					b.bucketHandleSetPushed = true
					b.lookupCtrlRef.PushBucketHandles(ctx, handles)
				}
				b.bucketHandleSetDirty = false
			}

			if stDirty {
				emitState()
			}

			// if necessary, start the lookup controller.
			if bc := b.bucketConf; bc != nil &&
				lookupCtrCancel == nil &&
				!bc.GetLookup().GetDisable() {
				var lookupCtrCtx context.Context
				lookupCtrCtx, lookupCtrCancel = context.WithCancel(ctx)
				go func() {
					_ = b.execLookupController(lookupCtrCtx, bc)
				}()
			}
		})
	}
}

// PushBlockStore pushes a new block store ID, triggering a bucket handle lookup.
//
// if reset is set, resets the routine if it already existed.
func (b *loadedBucket) PushBlockStore(blockStoreID string, reset bool) {
	b.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		_, existed := b.blockStores.SetKey(blockStoreID, true)
		if existed && reset {
			_, _ = b.blockStores.ResetRoutine(blockStoreID)
		}
	})
}

// ClearBlockStore clears a block store ID if it was previously pushed with PushBlockStore.
func (b *loadedBucket) ClearBlockStore(blockStoreID string) {
	b.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		removed := b.blockStores.RemoveKey(blockStoreID)
		if removed {
			b.bucketHandleSetDirty = true
			broadcast()
		}
	})
}

// GetLookup waits for the lookup.
func (b *loadedBucket) GetLookup(ctx context.Context) (bucket_lookup.Lookup, error) {
	return b.lookupCtr.WaitValue(ctx, nil)
}

// clearLookup removes the lookup from the lookupCh
func (b *loadedBucket) clearLookup() {
	b.lookupCtr.SetValue(nil)
}

// pushLookup pushes the lookup to the lookupCh
func (b *loadedBucket) pushLookup(l bucket_lookup.Lookup) {
	b.lookupCtr.SetValue(l)
}

// execLookupController manages a lookup controller instance.
// !bc.GetLookup().GetDisable asserted by caller
func (b *loadedBucket) execLookupController(
	ctx context.Context,
	bc *bucket.Config,
) (err error) {
	le := b.le.WithField("bucket-conf-rev", bc.GetRev())
	defer func() {
		if err != nil && err != context.Canceled {
			le.WithError(err).Warn("lookup controller exited with error")
		}
	}()
	// acquire controller conf
	c := bc.GetLookup().GetController()
	if c.GetId() == "" {
		// get default conf
		cc := b.c.cc
		if cc.GetDisableDefaultLookup() {
			return nil
		}
		c = cc.GetDefaultLookup()
	}
	var conf bucket_lookup.Config
	if c.GetId() == "" {
		conf = BuildDefaultLookupConfig()
	} else {
		cc, err := c.Resolve(ctx, b.c.b)
		if err != nil {
			return err
		}
		var ok bool
		conf, ok = cc.GetConfig().(bucket_lookup.Config)
		if !ok || conf == nil {
			return errors.Errorf(
				"config %s is not a bucket_lookup.Config",
				cc.GetConfig().GetConfigID(),
			)
		}
	}
	conf.SetBucketConf(bc)
	le = le.WithField("config-id", conf.GetConfigID())
	le.Debug("executing lookup controller")
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()
	var lastErr error
	di, diRef, err := b.c.b.AddDirective(
		resolver.NewLoadControllerWithConfig(conf),
		bus.NewCallbackHandler(
			func(av directive.AttachedValue) {
				lv, ok := av.GetValue().(resolver.LoadControllerWithConfigValue)
				if !ok {
					return
				}
				lvErr := lv.GetError()
				if lvErr != nil {
					if lastErr == lvErr {
						return
					}
					b.le.WithError(lvErr).Warn("lookup controller failed")
				}
				lastErr = lvErr
				var lc bucket_lookup.Controller
				if lvErr == nil {
					lc, _ = lv.GetController().(bucket_lookup.Controller)
				}
				b.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
					if b.bucketConf == bc && b.lookupCtrlRef != lc {
						b.lookupCtrlRef = lc
						b.bucketHandleSetPushed = false
						if lc != nil {
							b.le.Debug("lookup controller ready")
							b.pushLookup(lc)
						} else {
							b.le.Debug("lookup controller exited")
							b.clearLookup()
						}
						broadcast()
					}
				})
			}, func(av directive.AttachedValue) {
				lv, ok := av.GetValue().(resolver.LoadControllerWithConfigValue)
				if !ok {
					return
				}
				lc, ok := lv.GetController().(bucket_lookup.Controller)
				if !ok || lc == nil {
					return
				}
				b.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
					if b.lookupCtrlRef == lc {
						b.le.Debug("lookup controller exited")
						b.lookupCtrlRef = nil
						b.clearLookup()
						broadcast()
					}
				})
			},
			subCtxCancel,
		),
	)
	if err != nil {
		return err
	}
	defer diRef.Release()
	_ = di

	<-subCtx.Done()
	return nil
}
