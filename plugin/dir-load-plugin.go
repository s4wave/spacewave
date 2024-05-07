package bldr_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/routine"
)

// LoadPlugin is a directive to execute a plugin.
type LoadPlugin interface {
	// Directive indicates LoadPlugin is a directive.
	directive.Directive

	// LoadPluginID returns the plugin ID to load.
	// Cannot be empty.
	LoadPluginID() string
}

// LoadPluginValue is the result type for LoadPlugin.
// Multiple results may be pushed to the directive.
type LoadPluginValue = RunningPlugin

// loadPlugin implements LoadPlugin
type loadPlugin struct {
	pluginID string
}

// NewLoadPlugin constructs a new LoadPlugin directive.
func NewLoadPlugin(pluginID string) LoadPlugin {
	return &loadPlugin{pluginID: pluginID}
}

// ExLoadPlugin executes the LoadPlugin directive.
//
// if returnIfIdle=true and the directive becomes idle, returns nil, nil, nil
func ExLoadPlugin(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	pluginID string,
	valDisposeCallback func(),
) (LoadPluginValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[RunningPlugin](
		ctx,
		b,
		NewLoadPlugin(pluginID),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCallback,
		nil,
	)
}

// ExLoadPluginWaitClient calls LoadPlugin and returns the rpc client to be set.
// if returnIfIdle is set, returns nil, nil, nil if the directive becomes idle.
func ExPluginLoadWaitClient(
	ctx context.Context,
	b bus.Bus,
	pluginID string,
	valDisposeCb func(),
) (srpc.Client, directive.Reference, error) {
	rp, _, rpRef, err := ExLoadPlugin(ctx, b, false, pluginID, valDisposeCb)
	if err != nil || rp == nil {
		if rpRef != nil {
			rpRef.Release()
		}
		return nil, nil, err
	}

	return rp.GetRpcClient(), rpRef, nil
}

// ExLoadPluginAccess calls LoadPlugin and returns the running plugin handle.
//
// the callback will be canceled & restarted if the client becomes invalid.
// the callback context is canceled when the client value changes.
// the callback should return context.Canceled in that case.
//
// if the callback returns nil, the outer function will also return nil.
func ExPluginLoadAccess(
	ctx context.Context,
	b bus.Bus,
	pluginID string,
	cb func(ctx context.Context, rp RunningPlugin) error,
) error {
	routineCtr := routine.NewStateRoutineContainer(
		func(t1, t2 LoadPluginValue) bool { return t1 == t2 },
	)
	di, dirRef, err := bus.ExecOneOffWatchRoutine(
		routineCtr,
		b,
		NewLoadPlugin(pluginID),
	)
	if err != nil {
		return err
	}
	defer dirRef.Release()

	errCh := make(chan error, 1)
	defer di.AddIdleCallback(func(isIdle bool, resErrs []error) {
		if !isIdle {
			return
		}
		for _, err := range resErrs {
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
		}
	})()

	routineCtr.SetContext(ctx, true)
	routineCtr.SetStateRoutine(cb)
	return routineCtr.WaitExited(ctx, false, errCh)
}

// ExLoadPluginAccessClient calls LoadPlugin and returns the rpc client.
//
// the callback will be canceled & restarted if the client becomes invalid.
// the callback context is canceled when the client value changes.
// the callback should return context.Canceled in that case.
//
// if the callback returns nil, the outer function will also return nil.
func ExPluginLoadAccessClient(
	ctx context.Context,
	b bus.Bus,
	pluginID string,
	cb func(ctx context.Context, c srpc.Client) error,
) error {
	return ExPluginLoadAccess(ctx, b, pluginID, func(ctx context.Context, rp RunningPlugin) error {
		return cb(ctx, rp.GetRpcClient())
	})
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *loadPlugin) Validate() error {
	if d.pluginID == "" {
		return ErrEmptyPluginID
	}

	return nil
}

// GetValueLoadPluginOptions returns options relating to value handling.
func (d *loadPlugin) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// LoadPluginID returns the plugin ID.
func (d *loadPlugin) LoadPluginID() string {
	return d.pluginID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *loadPlugin) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LoadPlugin)
	if !ok {
		return false
	}

	if d.LoadPluginID() != od.LoadPluginID() {
		return false
	}

	return true
}

// GetName returns the directive's type name.
func (d *loadPlugin) GetName() string {
	return "LoadPlugin"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *loadPlugin) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["plugin-id"] = []string{d.LoadPluginID()}
	return vals
}

// _ is a type assertion
var (
	_ LoadPlugin                   = ((*loadPlugin)(nil))
	_ directive.DirectiveWithEquiv = ((*loadPlugin)(nil))
)
