package plugin_host

import (
	"context"
	"io"
	"sync/atomic"

	"github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
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
) (RunningPlugin, directive.Reference, error) {
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
		rp, rpRef, err := ExLoadPlugin(ctx, b, false, pluginID, func() {
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
// the callback context is canceled when the client value changes.
// the callback should return context.Canceled in that case.
// if the callback returns nil, the outer function will also return nil.
func ExPluginLoadAccessClient(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	pluginID string,
	cb func(ctx context.Context, client srpc.Client) error,
) error {
	var prevRpRef directive.Reference
	var prevRpRefCancel context.CancelFunc
	var prevWaitCh chan struct{}
	var clientCancel context.CancelFunc
	defer func() {
		if prevRpRef != nil {
			prevRpRef.Release()
		}
		if prevRpRefCancel != nil {
			prevRpRefCancel()
		}
		if clientCancel != nil {
			clientCancel()
		}
		if prevWaitCh != nil {
			<-prevWaitCh
		}
	}()
PluginLoop:
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		default:
		}

		pluginHandleCtx, pluginHandleCtxCancel := context.WithCancel(ctx)
		if prevRpRefCancel != nil {
			prevRpRefCancel()
		}
		prevRpRefCancel = pluginHandleCtxCancel

		var err error
		rp, rpRef, err := ExLoadPlugin(ctx, b, true, pluginID, pluginHandleCtxCancel)
		if prevRpRef != nil {
			prevRpRef.Release()
		}
		prevRpRef = rpRef
		if err != nil {
			return err
		}
		if rp == nil {
			return errors.Wrap(plugin.ErrNotFoundPlugin, pluginID)
		}

		clientCtr := rp.GetRpcClientCtr()
		var exit atomic.Bool
		var clientVal *srpc.Client
		var clientNonce atomic.Uint32
		errCh := make(chan error, 10)
		for {
			clientVal, err = clientCtr.WaitValueChange(pluginHandleCtx, clientVal, errCh)
			nextNonce := clientNonce.Add(1)
			if clientCancel != nil {
				clientCancel()
				clientCancel = nil
			}
			if err != nil {
				if exit.Load() {
					return nil
				}
				select {
				case <-ctx.Done():
					return context.Canceled
				case <-pluginHandleCtx.Done():
					continue PluginLoop
				default:
					return err
				}
			}

			if clientVal == nil {
				continue
			}

			if prevWaitCh != nil {
				select {
				case <-ctx.Done():
					return context.Canceled
				case <-prevWaitCh:
					prevWaitCh = nil
				}
			}

			var clientCtx context.Context
			clientCtx, clientCancel = context.WithCancel(pluginHandleCtx)
			prevWaitCh := make(chan struct{})
			go func(clientCtx context.Context, client srpc.Client, nonce uint32, doneCh chan struct{}) {
				defer close(doneCh)
				err := cb(clientCtx, client)
				if nonce != clientNonce.Load() {
					// ignore error, we canceled this instance.
					return
				}
				if err == nil {
					exit.Store(true)
					err = io.EOF
				}
				errCh <- err
			}(clientCtx, *clientVal, nextNonce, prevWaitCh)
		}
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *loadPlugin) Validate() error {
	if d.pluginID == "" {
		return plugin.ErrEmptyPluginID
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
