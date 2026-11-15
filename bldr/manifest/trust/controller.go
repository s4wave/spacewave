package bldr_manifest_trust

import (
	"context"
	"errors"
	"slices"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// ControllerID is the controller identifier.
const ControllerID = "bldr/manifest/trust"

// Version is the controller version.
var Version = controller.MustParseVersion("0.0.1")

// Controller resolves LookupTrustedRoots with one Set, and admits each
// AdmitPluginRoot whose plugin and root are in the Set, for any peer. The host
// constructs it in code, never from a config, so a config set cannot start a
// second one.
type Controller struct {
	// set is the trusted root set.
	set *Set
}

// NewController constructs a Controller serving set.
func NewController(set *Set) *Controller {
	return &Controller{set: set}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "trusted manifest roots")
}

// Execute executes the controller.
func (c *Controller) Execute(context.Context) error {
	return nil
}

// HandleDirective resolves LookupTrustedRoots with the set and AdmitPluginRoot
// once the root is in it.
func (c *Controller) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	switch dir := inst.GetDirective().(type) {
	case LookupTrustedRoots:
		return directive.R(directive.NewValueResolver([]LookupTrustedRootsValue{c.set}), nil)
	case bldr_plugin.AdmitPluginRoot:
		root := Root{PluginID: dir.AdmitPluginRootPluginID(), ManifestRoot: dir.AdmitPluginRootManifestRoot()}
		return directive.R(&admitResolver{set: c.set, root: root}, nil)
	}
	return nil, nil
}

// admitResolver admits one root once the add-only set holds it. A root never
// leaves the set, so the value is never removed.
type admitResolver struct {
	set  *Set
	root Root
}

// Resolve waits for the root to enter the set, then admits it.
func (r *admitResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Report idle while the root is absent, so a waiter sees no admission.
	if !r.set.Has(r.root) {
		handler.MarkIdle(true)
	}
	errFound := errors.New("found")
	err := r.set.Watch(ctx, func(added []Root) error {
		if slices.Contains(added, r.root) {
			return errFound
		}
		return nil
	})
	if err != errFound {
		return err
	}
	_, _ = handler.AddValue(bldr_plugin.AdmitSourceTrusted)
	return nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
