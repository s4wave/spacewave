package lookup_concurrent

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/blang/semver"
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
	// conf is the config
	conf *Config

	// bucketHandleSetCh contains the bucket handle set
	bucketHandleSetCh chan []volume.BucketHandle
}

// NewLookupController is the lookup controller constructor.
func NewLookupController(
	le *logrus.Entry,
	conf *Config,
) lookup.Controller {
	return &LookupController{
		le:                le.WithField("bucket-id", conf.GetBucketConf().GetId()),
		conf:              conf,
		bucketHandleSetCh: make(chan []volume.BucketHandle, 1),
	}
}

// Execute executes the reconciler controller.
func (c *LookupController) Execute(ctx context.Context) error {
	c.le.Info("executing concurrent bucket lookup controller")
	// TODO
	return nil
}

// LookupBlock searches for a block using the bucket lookup controller.
// If lookup is disabled, will return an error.
func (c *LookupController) LookupBlock(reqCtx context.Context, ref *cid.BlockRef) ([]byte, bool, error) {
	le := c.le.WithField("ref", ref.MarshalString())
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
	le.Debugf("checking %d handles", bhc)
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
			}
			return nil, false, rerr
		}
		return d, true, nil
	}
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
	c.le.Infof("got %d bucket handles", len(handles))
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
