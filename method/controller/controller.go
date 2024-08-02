package auth_method_controller

import (
	"context"
	"strings"

	auth_method "github.com/aperturerobotics/auth/method"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// MethodConstructor constructs an authentication method.
type MethodConstructor = auth_method.Constructor

// Controller implements a common auth method controller.
//
// The controller contains an authentication method and provides it on a bus.
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
	ctor MethodConstructor

	// methodCh holds the method like a bucket
	methodCh chan auth_method.Method
	// methodID is the method identifier.
	methodID string
	// methodVersion is the method version
	methodVersion semver.Version
}

// NewController constructs a new transport controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	ctor MethodConstructor,
	methodID string,
	methodVersion semver.Version,
) *Controller {
	return &Controller{
		le: le.WithField("auth-method-id", methodID).
			WithField("auth-method-version", methodVersion.String()),
		bus:  bus,
		ctor: ctor,

		methodCh:      make(chan auth_method.Method, 1),
		methodID:      methodID,
		methodVersion: methodVersion,
	}
}

// GetControllerID returns the controller ID.
func (c *Controller) GetControllerID() string {
	return strings.Join([]string{
		"aperture",
		"auth",
		"method",
		c.methodID,
		c.methodVersion.String(),
	}, "/")
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		c.GetControllerID(),
		c.methodVersion,
		"auth method controller "+c.methodID+"@"+c.methodVersion.String(),
	)
}

// Execute executes the auth method controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	c.ctx = ctx
	// Acquire a handle to the node.
	c.le.Debug("loading authentication method")

	// Construct the auth method.
	tpt, err := c.ctor(
		ctx,
		c.le,
		c,
	)
	if err != nil {
		return err
	}
	defer tpt.Close()
	c.methodCh <- tpt

	err = tpt.Execute(ctx)
	if err == nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		default:
			// wait to close the auth method until the ctx is canceled.
			<-ctx.Done()
			return nil
		}
	}
	select {
	case <-c.methodCh:
	default:
	}
	return err
}

// GetAuthMethod returns the controlled method.
func (c *Controller) GetAuthMethod(ctx context.Context) (auth_method.Method, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case tpt := <-c.methodCh:
		c.methodCh <- tpt
		return tpt, nil
	}
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case auth_method.AuthLookupMethod:
		return directive.R(c.resolveAuthLookupMethod(di, d))
	}

	return nil, nil
}

// HandleAuthMethodDiscoverKey handles an incoming private key discovery.
/*
func (c *Controller) HandleAuthMethodDiscoverKey(privKey crypto.PrivKey) error {
	// TODO
	return nil
}
*/

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	select {
	case tpt := <-c.methodCh:
		tpt.Close()
	default:
	}

	return nil
}

var (
	// _ is a type assertion
	_ controller.Controller = ((*Controller)(nil))
	// _ is a type assertion
	_ auth_method.Handler = ((*Controller)(nil))
)
