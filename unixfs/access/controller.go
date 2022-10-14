package unixfs_access

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/sirupsen/logrus"
)

// ControllerCallback is the callback to construct a handle.
// Returns the FSHandle, a release function, and error.
type ControllerCallback func(ctx context.Context) (*unixfs.FSHandle, func(), error)

// Controller wraps a handle constructor to resolve AccessUnixFS.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// fsId is the filesystem identifier
	fsId string
	// cb is the callback
	cb ControllerCallback
	// info is the controller info
	info *controller.Info
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	b bus.Bus,
	info *controller.Info,
	fsId string,
	cb ControllerCallback,
) *Controller {
	return &Controller{
		le:   le,
		b:    b,
		fsId: fsId,
		cb:   cb,
		info: info,
	}
}

// NewControllerWithHandle builds a new Controller which calls Clone on an
// existing handle.
func NewControllerWithHandle(
	le *logrus.Entry,
	b bus.Bus,
	info *controller.Info,
	fsId string,
	handle *unixfs.FSHandle,
) *Controller {
	return NewController(le, b, info, fsId, func(ctx context.Context) (*unixfs.FSHandle, func(), error) {
		fh, err := handle.Clone(ctx)
		if err != nil {
			return nil, nil, err
		}
		return fh, fh.Release, nil
	})
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *Controller) Execute(rctx context.Context) (rerr error) {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case AccessUnixFS:
		return directive.R(c.ResolveAccessUnixFS(ctx, inst, d))
	}
	return nil, nil
}

// AccessUnixFS accesses the filesystem.
func (c *Controller) AccessUnixFS(ctx context.Context) (*unixfs.FSHandle, func(), error) {
	if c.cb == nil {
		return nil, nil, errors.New("access unixfs callback is unset")
	}
	return c.cb(ctx)
}

// ResolveAccessUnixFS resolves an AccessUnixFS directive if the fs id matches.
func (c *Controller) ResolveAccessUnixFS(
	ctx context.Context,
	di directive.Instance,
	d AccessUnixFS,
) (directive.Resolver, error) {
	fsID := d.AccessUnixFSID()
	if c.fsId != fsID {
		return nil, nil
	}

	return directive.NewValueResolver([]AccessUnixFSValue{
		c.AccessUnixFS,
	}), nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
