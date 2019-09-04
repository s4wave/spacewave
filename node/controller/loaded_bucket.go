package node_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// loadedBucket contains state for a loaded bucket.
type loadedBucket struct {
	// ctxCancel is managed by controller.Execute
	ctxCancel context.CancelFunc

	c        *Controller
	le       *logrus.Entry
	bucketID string

	lookupCh chan bucket_lookup.Lookup
	wakeCh   chan struct{}

	mtx                  sync.Mutex
	ctx                  context.Context
	lastState            *loadedBucketState
	bucketConf           *bucket.Config
	lookupCtrlRef        bucket_lookup.Controller
	nrefID               uint32
	refs                 map[uint32]func(st *loadedBucketState)
	volumes              map[string]*loadedBucketVolume
	bucketHandleSetDirty bool
}

// loadedBucketState is a state callback payload for a loaded bucket.
type loadedBucketState struct {
	// ctx is expired when the state changes.
	ctx context.Context
	// ctxCancel cancels ctx
	ctxCancel context.CancelFunc
	// bucketConfig is the current known bucket config.
	bucketConfig *bucket.Config
}

// newLoadedBucket constructs a new loaded bucket.
func newLoadedBucket(c *Controller, bucketID string) *loadedBucket {
	return &loadedBucket{
		c:        c,
		le:       c.le.WithField("bucket-id", bucketID),
		bucketID: bucketID,
		lookupCh: make(chan bucket_lookup.Lookup, 1),
		wakeCh:   make(chan struct{}, 1),
		refs:     make(map[uint32]func(st *loadedBucketState)),
		volumes:  make(map[string]*loadedBucketVolume),
	}
}

// Execute executes the loaded bucket routine.
func (b *loadedBucket) Execute(ctx context.Context) error {
	b.le.Debug("starting bucket tracking")

	// State management routines.
	defer func() {
		b.mtx.Lock()
		if b.lastState != nil {
			b.lastState.ctxCancel()
		}
		for k, v := range b.volumes {
			delete(b.volumes, k)
			if v.ctxCancel != nil {
				v.ctxCancel()
			}
		}
		for k, ref := range b.refs {
			ref(nil)
			delete(b.refs, k)
		}
		b.mtx.Unlock()
		b.le.Debug("exited bucket tracking")
	}()

	var st loadedBucketState
	emitState := func() {
		if b.lastState != nil {
			b.lastState.ctxCancel()
		}
		ns := st
		b.lastState = &ns
		ns.ctx, ns.ctxCancel = context.WithCancel(ctx)
		for _, ref := range b.refs {
			ref(&ns)
		}
	}

	// startup
	b.mtx.Lock()
	b.ctx = ctx
	if len(b.volumes) != 0 {
		for _, v := range b.volumes {
			v.init(ctx)
		}
		b.wake()
	}
	b.mtx.Unlock()

	var lookupCtrCancel context.CancelFunc
	for {
		var stDirty bool
		select {
		case <-ctx.Done():
			if lookupCtrCancel != nil {
				lookupCtrCancel()
			}
			return ctx.Err()
		case <-b.wakeCh:
		}

		b.mtx.Lock()
		// gc, no references.
		if len(b.refs) == 0 {
			b.mtx.Unlock()
			if lookupCtrCancel != nil {
				lookupCtrCancel()
			}
			return nil
		}

		if st.bucketConfig != b.bucketConf {
			stDirty = true
			st.bucketConfig = b.bucketConf
			if lookupCtrCancel != nil {
				b.lookupCtrlRef = nil
				lookupCtrCancel()
				lookupCtrCancel = nil
				b.clearLookup()
			}
		}

		if b.bucketHandleSetDirty && b.lookupCtrlRef != nil {
			b.bucketHandleSetDirty = false
			handles := make([]volume.BucketHandle, 0, len(b.volumes))
			for _, v := range b.volumes {
				if v.bh != nil {
					handles = append(handles, v.bh)
				}
			}
			b.lookupCtrlRef.PushBucketHandles(ctx, handles)
		}

		if stDirty {
			emitState()
		}

		if bc := st.bucketConfig; bc != nil &&
			lookupCtrCancel == nil &&
			!bc.GetLookup().GetDisable() {
			var lookupCtrCtx context.Context
			lookupCtrCtx, lookupCtrCancel = context.WithCancel(ctx)
			go b.execLookupController(lookupCtrCtx, bc)
		}

		b.mtx.Unlock()
	}
}

// PushVolume pushes a new volume ID, triggering a bucket handle lookup.
func (b *loadedBucket) PushVolume(volumeID string) {
	b.mtx.Lock()
	if _, ok := b.volumes[volumeID]; !ok {
		nv := &loadedBucketVolume{
			b:        b,
			volumeID: volumeID,
		}
		nv.init(b.ctx)
		b.volumes[volumeID] = nv
		defer b.wake()
	}
	b.mtx.Unlock()
}

// ClearVolume clears a volume ID if it was previously pushed with PushVolume.
func (b *loadedBucket) ClearVolume(volumeID string) {
	b.mtx.Lock()
	if v, ok := b.volumes[volumeID]; ok {
		if v.ctxCancel != nil {
			v.ctxCancel()
			if v.bh != nil {
				v.bh = nil
				b.bucketHandleSetDirty = true
			}
			defer b.wake()
		}
		delete(b.volumes, volumeID)
	}
	b.mtx.Unlock()
}

// AddRef adds a reference to the loaded bucket.
func (b *loadedBucket) AddRef(cb func(s *loadedBucketState)) uint32 {
	b.mtx.Lock()
	b.nrefID++
	nrid := b.nrefID
	b.refs[nrid] = cb
	s := b.lastState
	if s == nil {
		b.wake()
	} else {
		cb(s)
	}
	b.mtx.Unlock()
	return nrid
}

// ClearRef clears a reference to the loaded bucket.
// Calls the callback with nil.
func (b *loadedBucket) ClearRef(id uint32) {
	b.mtx.Lock()
	if cb, ok := b.refs[id]; ok {
		cb(nil)
		delete(b.refs, id)
		if len(b.refs) == 0 {
			b.wake()
		}
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

// wake wakes the bucket executor.
func (b *loadedBucket) wake() {
	select {
	case b.wakeCh <- struct{}{}:
	default:
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
	di, diRef, err := b.c.b.AddDirective(
		resolver.NewLoadControllerWithConfig(conf),
		bus.NewCallbackHandler(
			func(av directive.AttachedValue) {
				lc, ok := av.GetValue().(bucket_lookup.Controller)
				if !ok {
					return
				}
				b.mtx.Lock()
				if b.bucketConf == bc {
					b.le.Debug("lookup controller ready")
					b.lookupCtrlRef = lc
					b.pushLookup(lc)
					defer b.wake()
				}
				b.mtx.Unlock()
			}, func(av directive.AttachedValue) {
				lc, ok := av.GetValue().(bucket_lookup.Controller)
				if !ok {
					return
				}
				b.mtx.Lock()
				if b.lookupCtrlRef == lc {
					b.le.Debug("lookup controller exited")
					b.lookupCtrlRef = nil
					b.clearLookup()
					defer b.wake()
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
