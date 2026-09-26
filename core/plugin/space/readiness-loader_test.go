package plugin_space

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
)

// pluginReadinessLoadController loads the readiness plugin and registers its
// object type only when the test allows it.
type pluginReadinessLoadController struct {
	// b is the bus the plugin loads on.
	b bus.Bus
	// loadStarted closes when LoadPlugin starts.
	loadStarted chan struct{}
	// manifestResolved closes when the plugin manifest resolves.
	manifestResolved chan struct{}
	// lookupObserved closes on the first LookupObjectType.
	lookupObserved chan struct{}
	// allowRegistration releases the object type registration.
	allowRegistration chan struct{}
	// lookupOnce closes lookupObserved.
	lookupOnce sync.Once
}

// newPluginReadinessLoadController constructs a loader on b.
func newPluginReadinessLoadController(b bus.Bus) *pluginReadinessLoadController {
	return &pluginReadinessLoadController{
		b:                 b,
		loadStarted:       make(chan struct{}),
		manifestResolved:  make(chan struct{}),
		lookupObserved:    make(chan struct{}),
		allowRegistration: make(chan struct{}),
	}
}

// GetControllerInfo returns information about the controller.
func (c *pluginReadinessLoadController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/plugin-readiness-loader",
		controller.MustParseVersion("0.0.1"),
		"loads a delayed test plugin",
	)
}

// Execute executes the controller.
func (c *pluginReadinessLoadController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// Close releases any resources used by the controller.
func (c *pluginReadinessLoadController) Close() error {
	return nil
}

// HandleDirective observes LookupObjectType and loads the readiness plugin.
func (c *pluginReadinessLoadController) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	if _, ok := inst.GetDirective().(objecttype.LookupObjectType); ok {
		c.lookupOnce.Do(func() {
			close(c.lookupObserved)
		})
		return nil, nil
	}
	load, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok || load.LoadPluginID() != pluginReadinessPluginID {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		close(c.loadStarted)
		manifestValue, _, manifestRef, err := bus.ExecWaitValue[*bldr_manifest.FetchManifestValue](
			ctx,
			c.b,
			bldr_manifest.NewFetchManifest(
				pluginReadinessPluginID,
				nil,
				[]string{pluginReadinessPlatformID},
				0,
			),
			func(_ bool, errs []error) (bool, error) {
				if len(errs) != 0 {
					return false, errs[0]
				}
				return true, nil
			},
			nil,
			nil,
		)
		if manifestRef != nil {
			defer manifestRef.Release()
		}
		if err != nil {
			return err
		}
		if len(manifestValue.GetManifestRefs()) == 0 {
			return nil
		}
		close(c.manifestResolved)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.allowRegistration:
		}
		registered := objecttype.NewObjectType(pluginReadinessTypeID, nil)
		objectTypeCtrl := objecttype_controller.NewController(func(
			_ context.Context,
			typeID string,
		) (objecttype.ObjectType, error) {
			if typeID != pluginReadinessTypeID {
				return nil, nil
			}
			return registered, nil
		})
		objectTypeRef, err := c.b.AddController(ctx, objectTypeCtrl, nil)
		if err != nil {
			return err
		}
		defer objectTypeRef()

		_, _ = handler.AddValue(bldr_plugin.NewRunningPlugin(nil))
		handler.MarkIdle(true)
		<-ctx.Done()
		return ctx.Err()
	}), nil)
}

// _ is a type assertion
var _ controller.Controller = (*pluginReadinessLoadController)(nil)
