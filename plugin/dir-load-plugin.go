package bldr_plugin

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
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

// RunningPlugin is the interface exposed to callers of LoadPlugin.
type RunningPlugin interface {
	// GetRpcClientCtr returns the rpc client container.
	// The plugin RPC client will be set when the plugin becomes ready.
	GetRpcClientCtr() *ccontainer.CContainer[*srpc.Client]
}

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
) (RunningPlugin, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[RunningPlugin](
		ctx,
		b,
		NewLoadPlugin(pluginID),
		returnIfIdle,
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
	var prevRpRef directive.Reference
	var currNonce atomic.Uint32
	var returned atomic.Bool
	for {
		nonce := currNonce.Add(1)
		select {
		case <-ctx.Done():
			if prevRpRef != nil {
				prevRpRef.Release()
			}
			return nil, nil, context.Canceled
		default:
		}
		waitCtx, waitCtxCancel := context.WithCancel(ctx)
		var err error
		rp, _, rpRef, err := ExLoadPlugin(ctx, b, false, pluginID, func() {
			waitCtxCancel()
			if valDisposeCb != nil && currNonce.Load() == nonce && returned.Load() {
				valDisposeCb()
			}
		})
		if prevRpRef != nil {
			prevRpRef.Release()
		}
		prevRpRef = rpRef
		if err != nil {
			waitCtxCancel()
			return nil, nil, err
		}
		// WaitValue waits for a non-nil client value.
		clientCtr := rp.GetRpcClientCtr()
		client, err := clientCtr.WaitValue(waitCtx, nil)
		waitCtxCancel()
		if err != nil {
			if err == context.Canceled {
				continue
			}
			rpRef.Release()
			return nil, nil, err
		}
		returned.Store(true)
		return *client, rpRef, nil
	}
}

// ExLoadPluginAccessClient calls LoadPlugin and returns the rpc client to be set.
// if returnIfIdle is set, returns ErrNotFoundPlugin if the directive becomes idle.
// the callback will be canceled & restarted if the client becomes invalid.
// the callback context is canceled when the client value changes.
// the callback should return context.Canceled in that case.
// if the callback returns nil, the outer function will also return nil.
func ExPluginLoadAccessClient(
	ctx context.Context,
	b bus.Bus,
	pluginID string,
	cb func(ctx context.Context, client srpc.Client) error,
) error {
	routineCtr, di, dirRef, err := bus.ExecOneOffWatchRoutine(
		b,
		NewLoadPlugin(pluginID),
		func(ctx context.Context, val LoadPluginValue) error {
			clientCtr := val.GetRpcClientCtr()
			clientPtr, err := clientCtr.WaitValue(ctx, nil)
			if err != nil {
				return err
			}
			return cb(ctx, *clientPtr)
		},
	)
	if err != nil {
		return err
	}
	defer dirRef.Release()

	errCh := make(chan error, 1)
	defer di.AddIdleCallback(func(resErrs []error) {
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
	return routineCtr.WaitExited(ctx, errCh)
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
