package reconciler_controller

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/reconciler"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/sirupsen/logrus"
)

// Controller implements a reconciler controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// reconciler contains the controlled reconciler
	reconciler reconciler.Reconciler
	// handleCtr contains the reconciler handle
	handleCtr *ccontainer.CContainer[*reconciler.Handle]
	// controllerInfo contains the controller info
	controllerInfo *controller.Info
}

// NewController constructs a new reconciler controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	info *controller.Info,
	rec reconciler.Reconciler,
) *Controller {
	return &Controller{
		le:             le,
		bus:            bus,
		controllerInfo: info,
		reconciler:     rec,
		handleCtr:      ccontainer.NewCContainer[*reconciler.Handle](nil),
	}
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	for {
		c.le.Debug("reconciler waiting for handle")
		handlePtr, err := c.handleCtr.WaitValue(ctx, nil)
		if err != nil {
			return err
		}

		// Execute the reconciler.
		c.le.Debug("reconciler executing")
		var exitedCleanly atomic.Bool
		recCtx, recCtxCancel := context.WithCancel(ctx)
		errCh := make(chan error, 1)
		go func(handle reconciler.Handle) {
			err := c.reconciler.Execute(recCtx, handle)
			if err == nil {
				exitedCleanly.Store(true)
				errCh <- context.Canceled
			} else {
				errCh <- err
			}
		}(*handlePtr)

		_, err = c.handleCtr.WaitValueChange(recCtx, handlePtr, errCh)
		recCtxCancel()
		if exitedCleanly.Load() {
			return nil
		}
		if err != nil && err != context.Canceled {
			return err
		}
	}
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return c.controllerInfo
}

// GetReconciler returns the reconciler instance.
func (c *Controller) GetReconciler() reconciler.Reconciler {
	return c.reconciler
}

// PushReconcilerHandle pushes the updated reconciler handle, overwriting
// any other pending handle. This will trigger a restart of the reconciler
// controller with the new handle.
func (c *Controller) PushReconcilerHandle(handle reconciler.Handle) {
	if handle == nil {
		c.handleCtr.SetValue(nil)
	} else {
		c.handleCtr.SetValue(&handle)
	}
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ reconciler.Controller = ((*Controller)(nil))
