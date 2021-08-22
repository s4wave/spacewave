package runtime_controller

import (
	"context"
	"strings"
	"sync"

	"github.com/aperturerobotics/bldr/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Constructor constructs a runtime with common parameters.
type Constructor func(
	ctx context.Context,
	le *logrus.Entry,
	handler runtime.RuntimeHandler,
) (runtime.Runtime, error)

// Controller implements a common bldr runtime controller.
// Tracks attached Runtime state and manages RPC calls in/out.
type Controller struct {
	// ctx is the controller context
	// set in the execute() function
	// ensure not used before execute sets it.
	ctx context.Context
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// ctor is the constructor
	ctor Constructor

	// runtimeID is the controller id to use
	runtimeID string
	// runtimeVersion is the version
	runtimeVersion semver.Version

	// trigger is pushed to when anything changes
	trigger chan struct{}
	// mtx guards the below fields
	mtx sync.Mutex
	// rt is the runtime
	rt runtime.Runtime
}

// NewController constructs a new transport controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	ctor Constructor,
	runtimeID string,
	runtimeVersion semver.Version,
) *Controller {
	return &Controller{
		le:   le,
		bus:  bus,
		ctor: ctor,

		runtimeID:      runtimeID,
		runtimeVersion: runtimeVersion,

		trigger: make(chan struct{}, 1),
	}
}

// GetControllerID returns the controller ID.
func (c *Controller) GetControllerID() string {
	return strings.Join([]string{
		"bldr",
		"runtime",
		c.runtimeID,
		c.runtimeVersion.String(),
	}, "/")
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() controller.Info {
	return controller.NewInfo(
		c.GetControllerID(),
		c.runtimeVersion,
		"bldr runtime controller "+c.runtimeID+"@"+c.runtimeVersion.String(),
	)
}

// Execute executes the runtime controller and the runtime itself.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	c.ctx = ctx
	defer ctxCancel()

	// Construct the runtime
	rt, err := c.ctor(
		ctx,
		c.le,
		c,
	)
	if err != nil {
		return err
	}
	defer func() {
		c.mtx.Lock()
		if c.rt == rt {
			c.rt = nil
			c.doTrigger()
		}
		c.mtx.Unlock()
		_ = rt.Close(ctx)
	}()

	c.mtx.Lock()
	c.rt = rt
	c.mtx.Unlock()
	c.doTrigger()

	c.le.Debug("executing bldr runtime")
	errCh := make(chan error, 1)
	go func() {
		errCh <- rt.Execute(ctx)
	}()

	// construct the storage providers
	storageProviders := rt.GetStorage()
	for _, st := range storageProviders {
		vc := st.BuildVolumeConfig("aperture")
		_, _, volRef, err := loader.WaitExecControllerRunning(
			ctx,
			c.bus,
			resolver.NewLoadControllerWithConfig(vc),
			nil,
		)
		if err != nil {
			return errors.Wrap(err, "start volume controller")
		}
		defer volRef.Release()
	}

	for {
		// TODO: query runtime view statuses ...

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			if err != nil {
				return err
			}
		}
	}
}

// GetRuntime returns the controlled runtime, waiting for it to be non-nil.
func (c *Controller) GetRuntime(ctx context.Context) (runtime.Runtime, error) {
	for {
		c.mtx.Lock()
		rt, trig := c.rt, c.trigger
		c.mtx.Unlock()
		if rt != nil {
			return rt, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-trig:
		}
	}
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) (directive.Resolver, error) {
	/* TODO
	dir := di.GetDirective()
	switch d := dir.(type) {
	case transport.LookupTransport:
		return c.resolveLookupTransport(ctx, di, d)
	}
	*/

	return nil, nil
}

// doTrigger triggers all waiting goroutines
func (c *Controller) doTrigger() {
	for {
		select {
		case c.trigger <- struct{}{}:
		default:
			return
		}
	}
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	c.mtx.Lock()
	c.ctor = nil
	c.rt = nil
	c.mtx.Unlock()
	return nil
}

// _ is a type assertion
var _ runtime.RuntimeController = ((*Controller)(nil))
