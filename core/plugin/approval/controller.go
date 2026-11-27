package plugin_approval

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller identifier.
const ControllerID = "spacewave/plugin/approval"

// Version is the controller version.
var Version = controller.MustParseVersion("0.0.1")

// Controller admits each AdmitPluginRoot whose peer allowed the root, and
// resolves LookupPluginApprovals with its Approvals. The host constructs it in
// code, so a config cannot start a second writer.
type Controller struct {
	// le is the logger.
	le *logrus.Entry
	// approvals holds the decisions.
	approvals *Approvals
}

// NewController constructs a Controller serving approvals. Several buses may
// each run a Controller over the same Approvals.
func NewController(le *logrus.Entry, approvals *Approvals) *Controller {
	return &Controller{le: le, approvals: approvals}
}

// GetApprovals returns the decisions the controller serves.
func (c *Controller) GetApprovals() *Approvals {
	return c.approvals
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "device plugin approvals")
}

// Execute executes the controller.
func (c *Controller) Execute(context.Context) error {
	return nil
}

// HandleDirective resolves AdmitPluginRoot and LookupPluginApprovals.
func (c *Controller) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	switch dir := inst.GetDirective().(type) {
	case bldr_plugin.AdmitPluginRoot:
		return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
			return c.resolveAdmit(ctx, handler, dir)
		}), nil)
	case LookupPluginApprovals:
		return directive.R(directive.NewValueResolver([]LookupPluginApprovalsValue{c.approvals}), nil)
	}
	return nil, nil
}

// resolveAdmit holds a value while the asking peer's record allows the root,
// rereading the record after each committed write. A read failure refuses.
func (c *Controller) resolveAdmit(ctx context.Context, handler directive.ResolverHandler, dir bldr_plugin.AdmitPluginRoot) error {
	// Read the root the directive asks about.
	peerID := dir.AdmitPluginRootPeerID()
	pluginID := dir.AdmitPluginRootPluginID()
	root := dir.AdmitPluginRootManifestRoot()

	// Reread the record after each committed write, holding a value only
	// while the record allows the root.
	var valID uint32
	for {
		// Take the wait channel before reading, so no write is missed.
		waitCh := c.approvals.WaitCh()
		admitted, err := c.approvals.Admits(ctx, peerID, pluginID, root)
		if err != nil {
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			c.le.WithError(err).WithFields(logrus.Fields{
				"plugin-id":     pluginID,
				"manifest-root": root,
			}).Warn("cannot read plugin approval; refusing")
		}

		// Publish the decision: a value only while admitted.
		switch {
		case admitted && valID == 0:
			id, accepted := handler.AddValue(bldr_plugin.AdmitSourceApproved)
			if accepted {
				valID = id
			}
		case !admitted && valID != 0:
			_, _ = handler.RemoveValue(valID)
			valID = 0
		}
		if !admitted {
			handler.MarkIdle(true)
		}

		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
