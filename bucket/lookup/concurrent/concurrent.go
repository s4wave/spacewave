package lookup_concurrent

import (
	"context"
	"sync"

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
	reqCtx context.Context,
	ref *block.BlockRef,
	optf ...lookup.LookupBlockOption,
) ([]byte, bool, error) {
	opts := lookup.NewLookupBlockOpts(optf...)
	if ref.GetEmpty() {
		return nil, false, block.ErrEmptyBlockRef
	}

	// acquire handles
	bh, err := c.getBucketHandles(reqCtx)
	if err != nil {
		return nil, false, err
	}

	le := func() *logrus.Entry {
		return c.le.WithField("ref", ref.MarshalString())
	}
	notFound := func() ([]byte, bool, error) {
		le().Debugf("ref not found against %d handles", len(bh))
		if c.conf.GetNotFoundBehavior() == NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE && !opts.LocalOnly {
			// NOTE: The controller implementing LookupBlockFromNetwork is also responsible for writing the found block
			// into one or more local volumes, as appropriate. If the controller that responds to LookupBlockFromNetwork
			// does not store the result in a local volume, then the directive will be fired on every lookup.
			return c.lookupWithDirective(reqCtx, ref)
		}
		return nil, false, nil
	}

	// fast path: only 0 or 1 bucket handle
	if len(bh) == 0 {
		return nil, false, errors.Wrap(bucket.ErrBucketUnknown, c.conf.GetBucketConf().GetId())
	}
	if len(bh) == 1 {
		d, ok, err := bh[0].GetBucket().GetBlock(ref)
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

	waitCh := bcast.GetWaitCh()
	running := len(bh)

	for _, hx := range bh {
		h := hx
		go func() {
			d, ok, err := h.GetBucket().GetBlock(ref)
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
	ctx context.Context,
	data []byte,
	opts *block.PutOpts,
) ([]*bucket.ObjectRef, bool, error) {
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	bucketHandles, err := c.getBucketHandles(subCtx)
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
		return h.GetBucket().PutBlock(data, opts)
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
			case <-subCtx.Done():
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
		case <-subCtx.Done():
			return nil, false, subCtx.Err()
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
func (c *LookupController) lookupWithDirective(reqCtx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	bucketID := c.conf.GetBucketConf().GetId()
	dir := dex.NewLookupBlockFromNetwork(bucketID, ref)
	subCtx, subCtxCancel := context.WithCancel(reqCtx)
	defer subCtxCancel()
	aval, _, aref, err := bus.ExecOneOff(subCtx, c.b, dir, false, nil)
	if err != nil {
		return nil, false, err
	}
	lval, ok := aval.GetValue().(dex.LookupBlockFromNetworkValue)
	aref.Release()
	if !ok {
		return nil, false, errors.New("dex lookup block from network returned invalid value")
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
// Any exceptional errors are returned for logging.
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
