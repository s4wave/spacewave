package node_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// loadedBucket contains state for a loaded bucket.
type loadedBucket struct {
	c        *Controller
	le       *logrus.Entry
	bucketID string

	stateCtr *ccontainer.CContainer[*loadedBucketState]
	lookupCh chan bucket_lookup.Lookup
	wake     broadcast.Broadcast

	mtx                   sync.Mutex
	lastState             *loadedBucketState
	bucketConf            *bucket.Config
	lookupCtrlRef         bucket_lookup.Controller
	nrefID                uint32
	volumes               *keyed.Keyed[string, *loadedBucketVolume]
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
		c:        c,
		le:       c.le.WithField("bucket-id", bucketID),
		bucketID: bucketID,
		lookupCh: make(chan bucket_lookup.Lookup, 1),
		stateCtr: ccontainer.NewCContainer[*loadedBucketState](nil),
	}
	lb.volumes = keyed.NewKeyed(lb.newLoadedBucketVolume)
	return lb.execute, lb
}

// execute executes the loaded bucket routine.
func (b *loadedBucket) execute(ctx context.Context) error {
	b.le.Debug("starting bucket tracking")

	// State management routines.
	defer b.volumes.SyncKeys(nil, false)
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
	var wakeCh <-chan struct{}
	b.volumes.SetContext(ctx, true)

	var lookupCtrCancel context.CancelFunc
	for {
		var stDirty bool

		if wakeCh != nil {
			select {
			case <-ctx.Done():
				if lookupCtrCancel != nil {
					lookupCtrCancel()
				}
				return ctx.Err()
			case <-wakeCh:
			}
		}

		b.mtx.Lock()
		wakeCh = b.wake.GetWaitCh()
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
			vols := b.volumes.GetKeysWithData()
			handles := make([]volume.BucketHandle, 0, len(vols))
			for _, vdat := range vols {
				v := vdat.Data
				if v != nil && v.bh != nil {
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

		b.mtx.Unlock()
	}
}

// PushVolume pushes a new volume ID, triggering a bucket handle lookup.
func (b *loadedBucket) PushVolume(volumeID string, reset bool) {
	b.mtx.Lock()
	_, existed := b.volumes.SetKey(volumeID, true)
	if existed && reset {
		_, _ = b.volumes.ResetRoutine(volumeID)
	}
	b.mtx.Unlock()
}

// ClearVolume clears a volume ID if it was previously pushed with PushVolume.
func (b *loadedBucket) ClearVolume(volumeID string) {
	b.mtx.Lock()
	removed := b.volumes.RemoveKey(volumeID)
	if removed {
		b.bucketHandleSetDirty = true
		b.wake.Broadcast()
	}
	b.mtx.Unlock()
}

// GetLookup waits for the lookup.
func (b *loadedBucket) GetLookup(ctx context.Context) (bucket_lookup.Lookup, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case l := <-b.lookupCh:
		select {
		case b.lookupCh <- l:
		default:
		}
		return l, nil
	}
}

// clearLookup removes the lookup from the lookupCh
func (b *loadedBucket) clearLookup() {
	for {
		select {
		case <-b.lookupCh:
		default:
			return
		}
	}
}

// pushLookup pushes the lookup to the lookupCh
func (b *loadedBucket) pushLookup(l bucket_lookup.Lookup) {
	for {
		select {
		case b.lookupCh <- l:
			return
		default:
		}
		select {
		case <-b.lookupCh:
		default:
		}
	}
}

// execLookupController manages a lookup controller instance.
// !bc.GetLookup().GetDisable asserted by caller
func (b *loadedBucket) execLookupController(
	ctx context.Context,
	bc *bucket.Config,
) (err error) {
	le := b.le.WithField("bucket-conf-ver", bc.GetVersion())
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
				b.mtx.Lock()
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
					b.wake.Broadcast()
				}
				b.mtx.Unlock()
			}, func(av directive.AttachedValue) {
				lv, ok := av.GetValue().(resolver.LoadControllerWithConfigValue)
				if !ok {
					return
				}
				lc, ok := lv.GetController().(bucket_lookup.Controller)
				if !ok || lc == nil {
					return
				}
				b.mtx.Lock()
				if b.lookupCtrlRef == lc {
					b.le.Debug("lookup controller exited")
					b.lookupCtrlRef = nil
					b.clearLookup()
					b.wake.Broadcast()
				}
				b.mtx.Unlock()
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
