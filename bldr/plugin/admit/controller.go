// Package plugin_admit answers a plugin host's AdmitPluginRoot asks through
// the plugin that owns the device's plugin approvals.
//
// Invariant: the host runs a root only when the trust set or this device's
// approvals, asked through their owner, admit it for the installation peer;
// while the owner is not running or has not answered, the load shows checking
// approval and runs nothing. The host never copies the approvals or reads
// their store: it holds a value only while the owner's stream says admitted.
//
// The host constructs the Controller in code, with the owner fixed from its
// own build facts, never from a config, so a config set cannot start a second
// one that names another plugin as the owner. Only a host whose owner runs
// as a separate plugin needs one: the dist entrypoint and the native and
// browser devtools. spacewave serve builds none, because its approvals
// controller runs on the serve bus itself and answers AdmitPluginRoot there.
package bldr_plugin_admit

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/plugin/admit"

// Version is the version of this controller.
var Version = controller.MustParseVersion("0.0.1")

// Controller resolves AdmitPluginRoot by asking the approvals' owner plugin.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the plugin host bus
	bus bus.Bus
	// ownerID is the plugin that owns the approvals and serves
	// PluginRootAdmission. Its own roots are never asked about.
	ownerID string
}

// NewController constructs the controller asking ownerID. The host calls it
// in code at each entrypoint; there is no factory.
func NewController(le *logrus.Entry, b bus.Bus, ownerID string) *Controller {
	return &Controller{le: le, bus: b, ownerID: ownerID}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"admits plugin roots via plugin: "+c.ownerID,
	)
}

// Execute executes the controller.
func (c *Controller) Execute(context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive. It leaves
// the owner's own roots and asks without a peer to the rest of the bus: the
// owner must run before it can answer, and its approvals are keyed by peer.
func (c *Controller) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(bldr_plugin.AdmitPluginRoot)
	if !ok || dir.AdmitPluginRootPluginID() == c.ownerID || dir.AdmitPluginRootPeerID() == "" {
		return nil, nil
	}
	return directive.R(directive.NewRetryResolver(
		c.le.WithField("via-plugin-id", c.ownerID),
		&admitResolver{c: c, dir: dir},
		newBackoff(),
	), nil)
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
