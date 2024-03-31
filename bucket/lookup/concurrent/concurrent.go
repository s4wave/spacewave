package lookup_concurrent

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/dex"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/conc"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID is the id for the concurrent lookup controller.
const ControllerID = "hydra/lookup/concurrent"

// Version is the version of the concurrent implementation.
var Version = semver.MustParse("0.0.1")

// LookupController implements the concurrent lookup controller.
type LookupController struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// conf is the config
	conf *Config

	// bucketHandleSetCtr contains the bucket handle set
	bucketHandleSetCtr *ccontainer.CContainer[*[]volume.BucketHandle]
}

// NewLookupController is the lookup controller constructor.
func NewLookupController(
	le *logrus.Entry,
	b bus.Bus,
	conf *Config,
) lookup.Controller {
	return &LookupController{
		le:                 le.WithField("bucket-id", conf.GetBucketConf().GetId()),
		b:                  b,
		conf:               conf,
		bucketHandleSetCtr: ccontainer.NewCContainer[*[]volume.BucketHandle](nil),
	}
}

// Execute executes the reconciler controller.
func (c *LookupController) Execute(ctx context.Context) error {
	return nil
}

// LookupBlock searches for a block using the bucket lookup controller.
// If lookup is disabled, will return an error.
func (c *LookupController) LookupBlock(
	rctx context.Context,
	ref *block.BlockRef,
	optf ...lookup.LookupBlockOption,
) (retData []byte, retFound bool, retErr error) {
	opts := lookup.NewLookupBlockOpts(optf...)
	if ref.GetEmpty() {
		return nil, false, block.ErrEmptyBlockRef
	}

	// apply lookup timeout
	var reqCtx context.Context
	var reqCtxCancel context.CancelFunc
	timeoutDur := opts.Timeout
	if timeoutDur == 0 {
		timeoutDur, _ = c.conf.ParseLookupTimeoutDur()
	}
	if timeoutDur > 0 {
		reqCtx, reqCtxCancel = context.WithTimeout(rctx, timeoutDur)
	} else {
		reqCtx, reqCtxCancel = context.WithCancel(rctx)
	}
	defer reqCtxCancel()

	// if timeout not found is set, transform DeadlineExceeded to not found.
	if opts.TimeoutNotFound {
		defer func() {
			if retErr == context.DeadlineExceeded {
				retErr, retFound, retData = nil, false, nil
			}
		}()
	}

	// acquire handles
	bh, err := c.getBucketHandles(reqCtx)
	if err != nil {
		return nil, false, err
	}

	le := func() *logrus.Entry {
		return c.le.WithField("ref", ref.MarshalString())
	}
	writeback := func(data []byte) error {
		if c.conf.GetWritebackBehavior() != WritebackBehavior_WritebackBehavior_ALL_VOLUMES {
			return nil
		}
		putOpts := &block.PutOpts{
			HashType:      ref.GetHash().GetHashType(),
			ForceBlockRef: ref,
		}
		doFns := make([]func(), 0, len(bh))
		var lastErr atomic.Pointer[error]
		var nw atomic.Uint32
		for _, h := range bh {
			if !h.GetExists() || h.GetBucket() == nil {
				continue
			}
			doFns = append(doFns, func() {
				_, existed, werr := h.GetBucket().PutBlock(reqCtx, data, putOpts)
				if werr != nil {
					lastErr.Store(&werr)
				} else if !existed {
					nw.Add(1)
				}
			})
		}
		if len(doFns) == 0 {
			return nil
		}
		q := conc.NewConcurrentQueue(runtime.NumCPU(), doFns...)
		if werr := q.WaitIdle(reqCtx, nil); werr != nil {
			return werr
		}
		if errp := lastErr.Load(); errp != nil {
			return *errp
		}
		if c.conf.GetVerbose() {
			if written := nw.Load(); written != 0 {
				le().Debugf("wrote-back block to %d handles", written)
			}
		}
		return nil
	}
	notFound := func() (data []byte, found bool, err error) {
		if c.conf.GetVerbose() {
			le().Debugf("ref not found against %d handles", len(bh))
		}
		notFoundBehavior := c.conf.GetNotFoundBehavior()
		var wait bool
		lookupDirective := notFoundBehavior == NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE
		if notFoundBehavior == NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE_WAIT {
			lookupDirective = true
			wait = true
		}
		if lookupDirective && !opts.LocalOnly {
			data, found, err = c.lookupWithDirective(reqCtx, ref, wait)
		}
		if found && err == nil {
			if werr := writeback(data); werr != nil {
				le().WithError(werr).Warn("unable to write-back block")
			}
		}
		return data, found, err
	}

	// fast path: only 0 or 1 bucket handle
	if len(bh) == 0 {
		return nil, false, errors.Wrap(bucket.ErrBucketNotFound, c.conf.GetBucketConf().GetId())
	}
	if len(bh) == 1 {
		d, ok, err := bh[0].GetBucket().GetBlock(reqCtx, ref)
		if err != nil {
			if err != context.Canceled {
				le().WithError(err).Warn("unable to lookup ref")
			}
			return nil, false, err
		}
		if !ok {
			return notFound()
		}
		return d, true, nil
	}

	// perform concurrent lookup
	var bcast broadcast.Broadcast
	var mtx sync.Mutex
	var rdata *[]byte
	var rerr error

	mtx.Lock()
	waitCh := bcast.GetWaitCh()
	var running int
	for _, hx := range bh {
		h := hx
		if !h.GetExists() {
			continue
		}
		running++
		go func() {
			d, ok, err := h.GetBucket().GetBlock(reqCtx, ref)
			mtx.Lock()
			if err != nil {
				// prioritize non context canceled errors
				if rerr == nil || err != context.Canceled {
					rerr = err
				}
			} else if ok && rdata == nil {
				rdata = &d
			}
			running--
			if running == 0 || rerr != nil || rdata != nil {
				bcast.Broadcast()
			}
			mtx.Unlock()
		}()
	}
	mtx.Unlock()

	select {
	case <-reqCtx.Done():
		return nil, false, context.Canceled
	case <-waitCh:
	}

	mtx.Lock()
	if rerr != nil {
		err := rerr
		mtx.Unlock()
		return nil, false, err
	}
	if rdata == nil {
		mtx.Unlock()
		return notFound()
	}
	rd := *rdata
	mtx.Unlock()
	return rd, true, nil
}

// PutBlock writes a block using the bucket lookup controller.
// The behavior of the write-back is configured in the lookup controller.
// If lookup is disabled, will return an error.
func (c *LookupController) PutBlock(
	reqCtx context.Context,
	data []byte, opts *block.PutOpts,
) ([]*bucket.ObjectRef, bool, error) {
	switch c.conf.GetPutBlockBehavior() {
	case PutBlockBehavior_PutBlockBehavior_ALL_VOLUMES:
		return c.putBlockAllVolumes(reqCtx, data, opts)
	default:
		return nil, false, nil
	}
}

// putBlockAllVolumes implements PutBlockBehavior_PutBlockBehavior_ALL_VOLUMES
func (c *LookupController) putBlockAllVolumes(
	rctx context.Context,
	data []byte,
	opts *block.PutOpts,
) ([]*bucket.ObjectRef, bool, error) {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	bucketHandles, err := c.getBucketHandles(ctx)
	if err != nil {
		return nil, false, err
	}
	type res struct {
		err error
		ex  bool
		e   *block.BlockRef
		b   string
	}
	resCh := make(chan *res)

	putBlockFn := func(h volume.BucketHandle) (bres *block.BlockRef, existed bool, berr error) {
		if !h.GetExists() {
			return nil, false, nil
		}
		return h.GetBucket().PutBlock(ctx, data, opts)
	}

	var br int
	for _, h := range bucketHandles {
		if !h.GetExists() {
			continue
		}
		br++
		go func(h volume.BucketHandle) {
			bres, existed, berr := putBlockFn(h)
			select {
			case <-ctx.Done():
				return
			case resCh <- &res{
				err: berr,
				e:   bres,
				ex:  existed,
				b:   h.GetID(),
			}:
			}
		}(h)
	}

	var rerr error
	refs := make([]*bucket.ObjectRef, 0, br)
	allExisted := true
	for i := 0; i < br; i++ {
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case res := <-resCh:
			if res.err != nil {
				if rerr == nil {
					rerr = res.err
				}
			} else if res.e != nil && !res.e.GetEmpty() {
				refs = append(refs, &bucket.ObjectRef{
					RootRef:  res.e,
					BucketId: res.b,
				})
				if !res.ex {
					allExisted = false
				}
			}
		}
	}
	return refs, allExisted, rerr
}

// lookupWithDirective uses the dex directive to lookup a block.
func (c *LookupController) lookupWithDirective(reqCtx context.Context, ref *block.BlockRef, wait bool) ([]byte, bool, error) {
	bucketID := c.conf.GetBucketConf().GetId()
	dir := dex.NewLookupBlockFromNetwork(bucketID, ref)

	var notFoundSeen atomic.Bool
	var idle atomic.Bool
	var idleCb bus.ExecIdleCallback = func(errs []error) (cwait bool, err error) {
		idle.Store(true)
		cwait, err = bus.ReturnIfIdle(!wait)(errs)
		if cwait && err == nil && notFoundSeen.Load() {
			// don't wait if we saw not-found or an error
			cwait = false
		}
		return cwait, err
	}

	lval, _, aref, err := bus.ExecWaitValue(
		reqCtx,
		c.b,
		dir,
		idleCb,
		nil,
		func(val dex.LookupBlockFromNetworkValue) (bool, error) {
			// if IgnoreNotFound is set in the lookup controller: prevents
			// resolving the directive with a not-found result.
			if err := val.GetError(); err != nil && err != block.ErrNotFound {
				return true, err
			}
			if len(val.GetData()) == 0 {
				notFoundSeen.Store(true)

				// if we already saw idle=true and not-found is returned,
				// return the not-found result immediately.
				return idle.Load(), nil
			}
			return true, nil
		},
	)
	if aref != nil {
		aref.Release()
	}
	if err != nil || lval == nil {
		return nil, false, err
	}

	return lval.GetData(), len(lval.GetData()) > 0 && lval.GetError() == nil, lval.GetError()
}

// getBucketHandles waits for the bucket handle set.
func (c *LookupController) getBucketHandles(ctx context.Context) ([]volume.BucketHandle, error) {
	valptr, err := c.bucketHandleSetCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return *valptr, nil
}

// PushBucketHandles pushes the bucket handle list that the controller may use
// to service requests. The controller should wait for this to be called before
// beginning to service requests. The bucket handles pushed should always have
// GetExists() == true.
func (c *LookupController) PushBucketHandles(ctx context.Context, handles []volume.BucketHandle) {
	c.bucketHandleSetCtr.SetValue(&handles)
}

// GetControllerInfo returns controller information.
func (c *LookupController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"bucket lookup "+c.conf.GetBucketConf().GetId(),
	)
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
// The context passed is canceled when the directive instance expires.
func (c *LookupController) HandleDirective(
	ctx context.Context,
	i directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *LookupController) Close() error {
	return nil
}

// _ is a type assertion
var _ lookup.Controller = ((*LookupController)(nil))
