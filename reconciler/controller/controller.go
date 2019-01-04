package reconciler_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/reconciler"
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
	// handleCh contains the reconciler handle
	handleCh chan reconciler.Handle
	// controllerInfo contains the controller info
	controllerInfo controller.Info
}

// NewController constructs a new reconciler controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	info controller.Info,
	rec reconciler.Reconciler,
) *Controller {
	return &Controller{
		le:             le,
		bus:            bus,
		controllerInfo: info,
		reconciler:     rec,
		handleCh:       make(chan reconciler.Handle, 1),
	}
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	c.le.Debug("reconciler waiting for handle")
	var handle reconciler.Handle
	select {
	case <-ctx.Done():
		return ctx.Err()
	case handle = <-c.handleCh:
		select {
		case c.handleCh <- handle:
		default:
		}
	}

	// Execute the reconciler.
	c.le.Debug("reconciler executing")
	handleCtx := handle.GetContext()
	recCtx, recCtxCancel := context.WithCancel(handleCtx)
	defer recCtxCancel()

	go func() {
		select {
		case <-ctx.Done():
			recCtxCancel()
		case <-recCtx.Done():
		}
	}()

	if err := c.reconciler.Execute(recCtx, handle); err != nil {
		return err
	}
	handle.FlushReconciler()
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) (directive.Resolver, error) {
	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() controller.Info {
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
	for {
		select {
		case c.handleCh <- handle:
			return
		default:
		}
		select {
		case <-c.handleCh:
		default:
		}
	}
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ reconciler.Controller = ((*Controller)(nil))
