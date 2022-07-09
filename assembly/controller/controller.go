package assembly_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bldr/assembly"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "controllerbus/assembly/1"

// Controller is the Assembly controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// c is the controller config
	c *Config
	// wakeCh wakes the controller
	wakeCh chan struct{}

	// mtx guards the below fields
	mtx        sync.Mutex
	assemblies []*runningAssembly
}

// NewController constructs a new peer controller.
// If privKey is nil, one will be generated.
func NewController(le *logrus.Entry, bus bus.Bus, cc *Config) (*Controller, error) {
	return &Controller{
		le:     le,
		bus:    bus,
		c:      cc,
		wakeCh: make(chan struct{}, 1),
	}, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"assembly controller",
	)
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
ExecLoop:
	for {
		select {
		case <-ctx.Done():
			break ExecLoop
		case <-c.wakeCh:
		}

		c.mtx.Lock()
		for _, runningAsm := range c.assemblies {
			if runningAsm.ctxCancel == nil {
				rctx, rctxCancel := context.WithCancel(ctx)
				runningAsm.ctxCancel = rctxCancel
				go runningAsm.Execute(rctx)
			}
		}
		c.mtx.Unlock()
	}

	c.mtx.Lock()
	asm := c.assemblies
	c.assemblies = nil
	c.mtx.Unlock()
	for _, a := range asm {
		if c := a.ctxCancel; c != nil {
			c()
		}
	}
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
	dir := di.GetDirective()
	switch d := dir.(type) {
	case assembly.ApplyAssembly:
		return c.resolveApplyAssembly(ctx, di, d), nil
	}

	return nil, nil
}

// PushAssembly pushes an assembly to run.
// returns nil, ErrEmptyAssembly if the conf was nil.
func (c *Controller) PushAssembly(
	ctx context.Context,
	conf assembly.Assembly,
) (assembly.Reference, error) {
	if conf == nil {
		return nil, assembly.ErrEmptyAssembly
	}

	c.mtx.Lock()
	asm, ref := newRunningAssembly(c, conf)
	c.assemblies = append(c.assemblies, asm)
	c.wake()
	c.mtx.Unlock()
	return ref, nil
}

// resolveApplyAssembly resolves the ApplyAssembly directive
func (c *Controller) resolveApplyAssembly(
	ctx context.Context,
	di directive.Instance,
	dir assembly.ApplyAssembly,
) directive.Resolver {
	return newApplyAssemblyResolver(c, ctx, di, dir)
}

// releaseAssembly removes an assembly from the ongoing set.
func (c *Controller) releaseAssembly(rc *runningAssembly) {
	c.mtx.Lock()
	for i, a := range c.assemblies {
		if a == rc {
			c.assemblies[i] = c.assemblies[len(c.assemblies)-1]
			c.assemblies[len(c.assemblies)-1] = nil
			c.assemblies = c.assemblies[:len(c.assemblies)-1]
			break
		}
	}
	c.mtx.Unlock()
}

// wake wakes the controller
func (c *Controller) wake() {
	select {
	case c.wakeCh <- struct{}{}:
	default:
	}
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ assembly.Controller = ((*Controller)(nil))
