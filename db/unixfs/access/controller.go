package unixfs_access

import (
	"context"
	"errors"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/sirupsen/logrus"
)

// Controller wraps a handle constructor to resolve AccessUnixFS.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// fsIds is the list of filesystem identifiers
	fsIds []string
	// cb is the callback
	cb AccessUnixFSValue
	// info is the controller info
	info *controller.Info
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	b bus.Bus,
	info *controller.Info,
	fsIds []string,
	cb AccessUnixFSValue,
) *Controller {
	return &Controller{
		le:    le,
		b:     b,
		fsIds: fsIds,
		cb:    cb,
		info:  info,
	}
}

// NewControllerWithHandle builds a new Controller which calls Clone on an
// existing handle.
func NewControllerWithHandle(
	le *logrus.Entry,
	b bus.Bus,
	info *controller.Info,
	fsIds []string,
	handle *unixfs.FSHandle,
) *Controller {
	return NewController(le, b, info, fsIds, func(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
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
func (c *Controller) AccessUnixFS(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
	if c.cb == nil {
		return nil, nil, errors.New("access unixfs callback is unset")
	}
	return c.cb(ctx, released)
}

// ResolveAccessUnixFS resolves an AccessUnixFS directive if the fs id matches.
func (c *Controller) ResolveAccessUnixFS(
	ctx context.Context,
	di directive.Instance,
	d AccessUnixFS,
) (directive.Resolver, error) {
	// Check if the requested fsID is in the list of supported fsIds.
	fsID := d.AccessUnixFSID()
	found := slices.Contains(c.fsIds, fsID)
	if !found {
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
