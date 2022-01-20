package bucket_setup

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"

	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "hydra/bucket/setup/1"

// Controller implements the bucket setup controller.
type Controller struct {
	// le is the log entry
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the configuration
	conf *Config
	// mtx guards below
	mtx sync.Mutex
	// closed indicates close was called
	closed bool
	// refs is the list of refs to dispose on close
	refs []func()
}

// NewController constructs a bucket setup controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	return &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() controller.Info {
	return controller.Info{
		Id:      ControllerID,
		Version: Version.String(),
	}
}

// Execute executes the controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	bucketConfs := c.conf.GetApplyBucketConfigs()
	refs := make([]func(), 0, len(bucketConfs)*2)
	running := int32(len(bucketConfs))

	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()
	errCh := make(chan error, 1)
	for i, conf := range bucketConfs {
		le := c.le.
			WithField("apply-bucket-config-idx", i).
			WithField("apply-bucket-config-id", conf.GetConfig().GetId())
		dir, err := conf.BuildDirective()
		if err != nil {
			le.WithError(err).Warn("apply bucket config was invalid")
			continue
		}
		di, ref, err := c.bus.AddDirective(dir, nil)
		if err != nil {
			le.WithError(err).Warn("apply bucket config failed")
			continue
		}
		refs = append(refs,
			di.AddIdleCallback(func(errs []error) {
				nrunning := atomic.AddInt32(&running, -1)
				if nrunning == 0 {
					subCtxCancel()
				}
				if len(errs) != 0 {
					select {
					case errCh <- errs[0]:
						return
					default:
					}
				}
			}),
			ref.Release,
		)
	}
	if len(refs) != 0 {
		c.le.Infof("applied %d bucket configs", len(refs))
	}
	c.mtx.Lock()
	closed := c.closed
	if !closed {
		c.refs = append(c.refs, refs...)
	}
	c.mtx.Unlock()
	if closed {
		for _, ref := range refs {
			ref()
		}
	}

	// wait
	select {
	case <-subCtx.Done():
		// return (become idle)
		return nil
	case err := <-errCh:
		return err
	}
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) (directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	// release all refs
	c.mtx.Lock()
	for _, r := range c.refs {
		r()
	}
	c.mtx.Unlock()
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
