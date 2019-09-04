package lookup_concurrent

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/dex"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the Example volume controller.
const ControllerID = "hydra/lookup/concurrent/1"

// Version is the version of the concurrent implementation.
var Version = semver.MustParse("0.0.1")

// LookupController implements a basic example reconciler.
type LookupController struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// conf is the config
	conf *Config

	// bucketHandleSetCh contains the bucket handle set
	bucketHandleSetCh chan []volume.BucketHandle
}

// NewLookupController is the lookup controller constructor.
func NewLookupController(
	le *logrus.Entry,
	b bus.Bus,
	conf *Config,
) lookup.Controller {
	return &LookupController{
		le:                le.WithField("bucket-id", conf.GetBucketConf().GetId()),
		b:                 b,
		conf:              conf,
		bucketHandleSetCh: make(chan []volume.BucketHandle, 1),
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
	ref *cid.BlockRef,
	optf ...lookup.LookupBlockOption,
) ([]byte, bool, error) {
	opts := lookup.NewLookupBlockOpts(optf...)

	// le := c.le.WithField("ref", ref.MarshalString())
	// acquire handles
	bh, err := c.getBucketHandles(reqCtx)
	if err != nil {
		return nil, false, err
	}

	dataCh := make(chan []byte, 1)
	var mtx sync.Mutex
	var rerr error

	// concurrently execute lookup
	// wait for first data OK to return
	// otherwise, if any errors, return them
	var wg sync.WaitGroup
	bhc := len(bh)
	wg.Add(bhc)
	for _, hx := range bh {
		h := hx
		go func() {
			defer wg.Done()
			d, ok, err := h.GetBucket().GetBlock(ref)
			if err != nil {
				mtx.Lock()
				if rerr == nil {
					rerr = err
				}
				mtx.Unlock()
				return
			}
			if ok {
				select {
				case dataCh <- d:
				default:
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(dataCh)
	}()

	select {
	case <-reqCtx.Done():
		return nil, false, reqCtx.Err()
	case d, ok := <-dataCh:
		if !ok {
			if rerr != nil {
				c.le.WithError(rerr).Warn("cannot lookup ref")
			} else {
				c.le.Debugf("ref not found against %d handles", bhc)
				if c.conf.GetNotFoundBehavior() == NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE && !opts.LocalOnly {
					return c.lookupWithDirective(reqCtx, ref)
				}
			}
			return nil, false, rerr
		}
		return d, true, nil
	}
}

// lookupWithDirective uses the dex directive to lookup a block.
func (c *LookupController) lookupWithDirective(reqCtx context.Context, ref *cid.BlockRef) ([]byte, bool, error) {
	bucketID := c.conf.GetBucketConf().GetId()
	dir := dex.NewLookupBlockFromNetwork(bucketID, ref)
	subCtx, subCtxCancel := context.WithCancel(reqCtx)
	defer subCtxCancel()
	aval, aref, err := bus.ExecOneOff(subCtx, c.b, dir, nil)
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
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case h := <-c.bucketHandleSetCh:
		select {
		case c.bucketHandleSetCh <- h:
		default:
		}
		return h, nil
	}
}

// PushBucketHandles pushes the bucket handle list that the controller may
// use to service requests. The controller should wait for this to be called
// before beginning to service requests. The bucket handles pushed will
// ys have GetExists() == true.
func (c *LookupController) PushBucketHandles(ctx context.Context, handles []volume.BucketHandle) {
	for {
		select {
		case c.bucketHandleSetCh <- handles:
			return
		default:
		}
		select {
		case <-c.bucketHandleSetCh:
		default:
		}
	}
}

// GetControllerInfo returns controller information.
func (c *LookupController) GetControllerInfo() controller.Info {
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
) (directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *LookupController) Close() error {
	return nil
}

// _ is a type assertion
var _ lookup.Controller = ((*LookupController)(nil))
