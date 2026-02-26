package bucket_setup

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"

	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "hydra/bucket/setup"

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
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"bucket setup controller",
	)
}

// Execute executes the controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	bucketConfs := c.conf.GetApplyBucketConfigs()
	refs := make([]func(), 0, len(bucketConfs)*2)
	var running atomic.Int32
	running.Store(int32(len(bucketConfs))) //nolint:gosec

	errCh := make(chan error, 1)
	handleErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	for i, conf := range bucketConfs {
		if conf.GetConfig() == nil {
			continue
		}
		le := c.le.
			WithField("apply-bucket-config-idx", i).
			WithField("apply-bucket-config-id", conf.GetConfig().GetId())
		conf = conf.CloneVT()
		if conf.GetConfig().GetRev() == 0 {
			conf.Config.Rev = 1
		}
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
		var markedNotRunning atomic.Bool
		refs = append(refs,
			di.AddIdleCallback(func(isIdle bool, errs []error) {
				for _, err := range errs {
					if err != nil && err != context.Canceled {
						le.WithError(err).Warn("apply bucket config failed")
						handleErr(err)
						return
					}
				}
				if !isIdle || markedNotRunning.Swap(true) {
					return
				}
				if running.Add(-1) == 0 {
					handleErr(nil)
				}
			}),
			ref.Release,
		)
	}
	if len(refs) != 0 {
		c.le.Infof("applied %d bucket configs", len(refs)/2)
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
	case <-ctx.Done():
		// return (become idle)
		return context.Canceled
	case err := <-errCh:
		return err
	}
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// release all refs
	for _, r := range c.refs {
		r()
	}
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
